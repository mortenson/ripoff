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
	"syscall"
	"time"
)

type Request struct {
	Type      string   `json:"type"`
	ValueFunc string   `json:"valueFunc"`
	Args      []string `json:"args"`
}

type Response struct {
	Value string `json:"value"`
}

type PluginResponse struct {
	response Response
	err      error
}

type PluginCall struct {
	plugin       RipoffPlugin
	valueFunc    string
	args         []string
	responseChan chan PluginResponse
}

type PluginManager struct {
	valueFuncMap    map[string]RipoffPlugin
	spawnedCommands []*exec.Cmd
	addressToConn   map[string]net.Conn
	callChan        chan PluginCall
}

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

func (m *PluginManager) Supports(valueFunc string) bool {
	_, ok := m.valueFuncMap[valueFunc]
	return ok
}

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
	timer := time.AfterFunc(5*time.Second, func() {
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

func connect(address string) (net.Conn, error) {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (m *PluginManager) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case call := <-m.callChan:
			conn, hasCon := m.addressToConn[call.plugin.Address]
			if !hasCon {
				call.responseChan <- PluginResponse{err: fmt.Errorf("connection for plugin %s does not exist", strings.Join(call.plugin.Command, " "))}
				return
			}
			// Send message to open TCP socket
			err := conn.SetReadDeadline(time.Now().Add(time.Second * 1))
			if err != nil {
				slog.Error("Could not set read deadline for plugin connection", slog.Any("error", err))
			}
			scanner := bufio.NewScanner(conn)
			message, err := json.Marshal(Request{
				Type:      "valueFunc",
				ValueFunc: call.valueFunc,
				Args:      call.args,
			})
			if err != nil {
				call.responseChan <- PluginResponse{err: err}
				return
			}
			_, err = conn.Write(append(message, '\n'))
			if err != nil {
				call.responseChan <- PluginResponse{err: err}
				return
			}
			if !scanner.Scan() {
				call.responseChan <- PluginResponse{err: fmt.Errorf("plugin command '%s' failed to respond to TCP message: %v", strings.Join(call.plugin.Command, " "), scanner.Err())}
				return
			}
			line := scanner.Bytes()
			response := Response{}
			err = json.Unmarshal(line, &response)
			if err != nil {
				call.responseChan <- PluginResponse{err: err}
				return
			}
			call.responseChan <- PluginResponse{response: response, err: nil}
		}
	}
}

func (m *PluginManager) Call(valueFunc string, args ...string) (string, error) {
	plugin, hasPlugin := m.valueFuncMap[valueFunc]
	if !hasPlugin {
		return "", fmt.Errorf("plugin for valueFunc %s is not defined", valueFunc)
	}
	responseChan := make(chan PluginResponse, 1)
	m.callChan <- PluginCall{
		plugin:       plugin,
		valueFunc:    valueFunc,
		args:         args,
		responseChan: responseChan,
	}
	response := <-responseChan
	return response.response.Value, response.err
}

func NewPluginManager(ctx context.Context, plugins map[string]RipoffPlugin) (*PluginManager, error) {
	m := &PluginManager{
		valueFuncMap:    map[string]RipoffPlugin{},
		spawnedCommands: []*exec.Cmd{},
		callChan:        make(chan PluginCall),
		addressToConn:   map[string]net.Conn{},
	}
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
		cmd, err := spawn(plugin.Command)
		if err != nil {
			return nil, err
		}
		m.spawnedCommands = append(m.spawnedCommands, cmd)
		conn, err := connect(plugin.Address)
		if err != nil {
			return nil, err
		}
		m.addressToConn[plugin.Address] = conn
	}
	go m.Run(ctx)
	return m, nil
}
