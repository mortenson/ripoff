package ripoff

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
)

const PLUGIN_STARTUP_DEADLINE = 5 * time.Second
const PLUGIN_TCP_CONNECTION_DEADLINE = time.Second

// The shape that plugins expect for requests
type Request struct {
	Id        string   `json:"id"`
	Type      string   `json:"type"`
	ValueFunc string   `json:"valueFunc"`
	Args      []string `json:"args"`
}

// The shape that plugins expect for responses
type Response struct {
	Id    string `json:"id"`
	Value string `json:"value"`
}

// Used to communicate async responses to a goroutine that sends them syncronously to plugins
type ResponseChanMessage struct {
	response Response
	err      error
}

// Used by a goroutine that sends messages over a response channel
type CallChanMessage struct {
	plugin       RipoffPlugin
	valueFunc    string
	args         []string
	responseChan chan ResponseChanMessage
}

// Manages plugin commands and TCP connections - intended to be used as a singleton for the entire ripoff process.
type PluginManager struct {
	valueFuncMap    map[string]RipoffPlugin
	spawnedCommands []*exec.Cmd
	addressToConn   map[string]net.Conn
	callChan        chan CallChanMessage
}

// Closes all open connections and kills process group for each plugin command and its children.
func (m *PluginManager) Close() {
	close(m.callChan)
	for _, conn := range m.addressToConn {
		err := conn.Close()
		if err != nil {
			slog.Error("Could not close plugin connection", slog.Any("error", err))
		}
	}
	for _, cmd := range m.spawnedCommands {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if err != nil {
			slog.Error("Could not kill process", slog.Any("command", cmd), slog.Any("error", err))
		}
	}
}

// Determines if a plugin provides the given valueFunc
func (m *PluginManager) Supports(valueFunc string) bool {
	_, ok := m.valueFuncMap[valueFunc]
	return ok
}

// Spawns a new plugin and waits for it to be ready
func spawn(command []string) (*exec.Cmd, error) {
	commandArgs := []string{}
	if len(command) > 1 {
		commandArgs = command[1:]
	}
	cmd := exec.Command(command[0], commandArgs...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	err = cmd.Start()
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Scan()
	line := scanner.Text()
	// Set deadline for outputting READY message
	timer := time.AfterFunc(PLUGIN_STARTUP_DEADLINE, func() {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if err != nil {
			slog.Error("Could not kill plugin after READY timeout", slog.Any("error", err))
		}
	})
	if !strings.Contains(string(line), "READY") {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if err != nil {
			slog.Error("Could not kill plugin after seeing it did not output READY", slog.Any("error", err))
		}
		return nil, fmt.Errorf("plugin command '%s' failed to output READY. Got: '%s' instead", strings.Join(command, " "), line)
	}
	// Stop the timeout kill
	timer.Stop()
	return cmd, nil
}

// Initializes a connection to the given TCP address.
func connect(address string) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", address, PLUGIN_TCP_CONNECTION_DEADLINE)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// Starts goroutines for the plugin manager, which mostly handle TCP requests and responses.
func (m *PluginManager) run(ctx context.Context) {
	// A sync map used to associate arbitrary responses with stalled goroutines, based on a random ID from a request message.
	var idMap sync.Map
	for _, conn := range m.addressToConn {
		// Watch for new responses from this plugin.
		// Should hopefully be halted when the connection is closed.
		go func() {
			scanner := bufio.NewScanner(conn)
			for scanner.Scan() {
				line := scanner.Bytes()
				response := Response{}
				err := json.Unmarshal(line, &response)
				if err != nil {
					slog.Error("Unable to parse response", slog.Any("error", err))
					continue
				}
				responseChanMessage, ok := idMap.Load(response.Id)
				if !ok {
					slog.Error("No plugin channel found in map for response ID", slog.Any("line", line))
					continue
				}
				// The goroutine that sent the request is waiting for a response
				responseChanMessage.(chan ResponseChanMessage) <- ResponseChanMessage{response: response, err: nil}
			}
		}()
	}
	for {
		select {
		case <-ctx.Done():
			return
		// New request to send to a plugin.
		case call := <-m.callChan:
			conn, hasCon := m.addressToConn[call.plugin.Address]
			if !hasCon {
				call.responseChan <- ResponseChanMessage{err: fmt.Errorf("connection for plugin %s does not exist", strings.Join(call.plugin.Command, " "))}
				return
			}
			// Generate a random ID to associate responses with this request.
			id := uuid.New().String()
			idMap.Store(id, call.responseChan)
			message, err := json.Marshal(Request{
				Id:        id,
				Type:      "valueFunc",
				ValueFunc: call.valueFunc,
				Args:      call.args,
			})
			if err != nil {
				call.responseChan <- ResponseChanMessage{err: err}
				return
			}
			_, err = conn.Write(append(message, '\n'))
			if err != nil {
				call.responseChan <- ResponseChanMessage{err: err}
				return
			}
		}
	}
}

// Calls an arbitrary plugin associated with this valueFunc over TCP.
func (m *PluginManager) Call(valueFunc string, args ...string) (string, error) {
	plugin, hasPlugin := m.valueFuncMap[valueFunc]
	if !hasPlugin {
		return "", fmt.Errorf("plugin for valueFunc %s is not defined", valueFunc)
	}
	// Create a channel that can be used to resume this function
	responseChan := make(chan ResponseChanMessage, 1)
	m.callChan <- CallChanMessage{
		plugin:       plugin,
		valueFunc:    valueFunc,
		args:         args,
		responseChan: responseChan,
	}
	// Block as we wait for a response
	response := <-responseChan
	return response.response.Value, response.err
}

func NewPluginManager(ctx context.Context, plugins map[string]RipoffPlugin) (*PluginManager, error) {
	m := &PluginManager{
		valueFuncMap:    map[string]RipoffPlugin{},
		spawnedCommands: []*exec.Cmd{},
		callChan:        make(chan CallChanMessage),
		addressToConn:   map[string]net.Conn{},
	}
	// Store a map of valueFuncs to plugins and also validate that there is no overlap.
	for pluginName, plugin := range plugins {
		if len(plugin.Command) == 0 {
			return nil, fmt.Errorf("cannot create new PluginManager - the plugin %s does not define a command", pluginName)
		}
		for _, valueFunc := range plugin.ValueFuncs {
			_, alreadySet := m.valueFuncMap[valueFunc]
			if alreadySet {
				return nil, fmt.Errorf("cannot create new PluginManager - the valueFunc %s is set in more than one plugin", valueFunc)
			}
			m.valueFuncMap[valueFunc] = plugin
		}
	}
	for _, plugin := range plugins {
		// Startup the plugin
		cmd, err := spawn(plugin.Command)
		if err != nil {
			return nil, err
		}
		m.spawnedCommands = append(m.spawnedCommands, cmd)
		// Connect to the plugin's address over TCP
		conn, err := connect(plugin.Address)
		if err != nil {
			return nil, err
		}
		m.addressToConn[plugin.Address] = conn
	}
	go m.run(ctx)
	return m, nil
}
