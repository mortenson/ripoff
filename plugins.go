package ripoff

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

type PluginManager struct {
	valueFuncMap    map[string]RipoffPlugin
	spawnedCommands []*exec.Cmd
	addressToConn   map[string]net.Conn
}

func (m *PluginManager) Close() {
	for _, conn := range m.addressToConn {
		message, _ := json.Marshal(Request{
			Type: "exit",
		})
		_, err := conn.Write(append(message, '\n'))
		if err != nil {
			slog.Error("Could not write exit message to plugn connection", slog.Any("error", err))
		}
		err = conn.Close()
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

type Request struct {
	Type      string   `json:"type"`
	ValueFunc string   `json:"valueFunc"`
	Args      []string `json:"args"`
}

type Response struct {
	Value string `json:"value"`
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

func (m *PluginManager) Call(valueFunc string, args ...string) (string, error) {
	plugin, hasPlugin := m.valueFuncMap[valueFunc]
	if !hasPlugin {
		return "", fmt.Errorf("plugin for valueFunc %s is not defined", valueFunc)
	}
	conn, ok := m.addressToConn[plugin.Address]
	// Attempt to start process and wait for port to open
	if !ok {
		cmd, err := spawn(plugin.Command)
		if err != nil {
			return "", err
		}
		m.spawnedCommands = append(m.spawnedCommands, cmd)
		conn, err = connect(plugin.Address)
		if err != nil {
			return "", err
		}
		m.addressToConn[plugin.Address] = conn
	}
	// Send message to open TCP socket
	err := conn.SetReadDeadline(time.Now().Add(time.Second))
	if err != nil {
		slog.Error("Could not set read deadline for plugin connection", slog.Any("error", err))
	}
	scanner := bufio.NewScanner(conn)
	message, err := json.Marshal(Request{
		Type:      "valueFunc",
		ValueFunc: valueFunc,
		Args:      args,
	})
	if err != nil {
		return "", err
	}
	_, err = conn.Write(append(message, '\n'))
	if err != nil {
		return "", err
	}
	if !scanner.Scan() {
		return "", fmt.Errorf("plugin command '%s' failed to response to TCP message: %v", strings.Join(plugin.Command, " "), scanner.Err())
	}
	line := scanner.Bytes()
	response := Response{}
	err = json.Unmarshal(line, &response)
	if err != nil {
		return "", err
	}
	return response.Value, nil
}

func NewPluginManager(plugins map[string]RipoffPlugin) (*PluginManager, error) {
	m := &PluginManager{
		valueFuncMap:    map[string]RipoffPlugin{},
		addressToConn:   map[string]net.Conn{},
		spawnedCommands: []*exec.Cmd{},
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
	return m, nil
}
