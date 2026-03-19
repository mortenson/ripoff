package ripoff

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

type PluginManager struct {
	valueFuncMap    map[string]RipoffPlugin
	spawnedCommands []*exec.Cmd
	connections     map[string]net.Conn
}

func (m *PluginManager) Close() {
	// Ignore errors
	for _, conn := range m.connections {
		message, _ := json.Marshal(Request{
			Type: "exit",
		})
		conn.Write(append(message, '\n'))
		conn.Close()
	}
	for _, cmd := range m.spawnedCommands {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
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
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	})
	if !strings.Contains(string(line), "READY") {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		return nil, fmt.Errorf("Plugin command '%s' failed to output READY. Got: '%s' instead", strings.Join(command, " "), line)
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
		return "", fmt.Errorf("Plugin for valueFunc %s is not defined", valueFunc)
	}
	conn, ok := m.connections[valueFunc]
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
		m.connections[valueFunc] = conn
	}
	// Send message to open TCP socket
	conn.SetReadDeadline(time.Now().Add(time.Second))
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
		return "", fmt.Errorf("Plugin command '%s' failed to response to TCP message: %v", strings.Join(plugin.Command, " "), scanner.Err())
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
		connections:     map[string]net.Conn{},
		spawnedCommands: []*exec.Cmd{},
	}
	for pluginName, plugin := range plugins {
		if len(plugin.Command) == 0 {
			return nil, fmt.Errorf("Cannot create new PluginManager - the plugin %s does not define a command.", pluginName)
		}
		for _, valueFunc := range plugin.ValueFuncs {
			_, alreadySet := m.valueFuncMap[valueFunc]
			if alreadySet {
				return nil, fmt.Errorf("Cannot create new PluginManager - the valueFunc %s is set in more than one plugin.", valueFunc)
			}
			m.valueFuncMap[valueFunc] = plugin
		}
	}
	return m, nil
}
