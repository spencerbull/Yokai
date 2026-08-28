package agent

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"sync"

	"github.com/spencerbull/yokai/internal/deployments"
)

type dockerLogTailOutput struct {
	Data      []byte
	Truncated bool
}

type dockerLogTailRunner func(context.Context, string, int) (dockerLogTailOutput, error)

type boundedTailWriter struct {
	mu        sync.Mutex
	buffer    []byte
	limit     int
	truncated bool
}

func newBoundedTailWriter(limit int) *boundedTailWriter {
	if limit < 0 {
		limit = 0
	}
	return &boundedTailWriter{buffer: make([]byte, 0, limit), limit: limit}
}

func (writer *boundedTailWriter) Write(data []byte) (int, error) {
	written := len(data)
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if writer.limit == 0 {
		writer.truncated = writer.truncated || len(data) > 0
		return written, nil
	}
	if len(data) >= writer.limit {
		writer.truncated = writer.truncated || len(writer.buffer) > 0 || len(data) > writer.limit
		writer.buffer = writer.buffer[:writer.limit]
		copy(writer.buffer, data[len(data)-writer.limit:])
		return written, nil
	}
	if overflow := len(writer.buffer) + len(data) - writer.limit; overflow > 0 {
		copy(writer.buffer, writer.buffer[overflow:])
		writer.buffer = writer.buffer[:len(writer.buffer)-overflow]
		writer.truncated = true
	}
	writer.buffer = append(writer.buffer, data...)
	return written, nil
}

func (writer *boundedTailWriter) output() dockerLogTailOutput {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	data := make([]byte, len(writer.buffer))
	copy(data, writer.buffer)
	return dockerLogTailOutput{Data: data, Truncated: writer.truncated}
}

func runDockerLogTail(ctx context.Context, id string, lines int) (dockerLogTailOutput, error) {
	output := newBoundedTailWriter(deployments.MaxLogTailBytes)
	command := exec.CommandContext(ctx, "docker", "logs", "--tail", strconv.Itoa(lines), id)
	command.Stdout = output
	command.Stderr = output
	err := command.Run()
	if err != nil {
		return output.output(), fmt.Errorf("docker logs failed: %w", err)
	}
	return output.output(), nil
}

func captureContainerLogTail(ctx context.Context, id, exactRedaction string, run dockerLogTailRunner) (deployments.LogTailCapture, error) {
	output, err := run(ctx, id, deployments.MaxLogTailLines)
	if err != nil && len(output.Data) == 0 {
		return deployments.LogTailCapture{}, err
	}
	partial := err != nil
	if output.Truncated {
		output.Data = discardPotentiallyPartialLogLine(output.Data)
	}
	tail, truncated := deployments.SanitizeLogTail(string(output.Data), exactRedaction)
	return deployments.LogTailCapture{Tail: tail, Truncated: partial || output.Truncated || truncated}, nil
}

func discardPotentiallyPartialLogLine(data []byte) []byte {
	for index, value := range data {
		if value != '\n' && value != '\r' {
			continue
		}
		index++
		if index < len(data) && value == '\r' && data[index] == '\n' {
			index++
		}
		return data[index:]
	}
	return nil
}
