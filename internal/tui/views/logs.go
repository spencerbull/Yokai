package views

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/tui/components"
	"github.com/spencerbull/yokai/internal/tui/theme"
)

// logLineMsg is sent when a new log line is received from SSE.
type logLineMsg struct {
	Line string
}

// logErrorMsg is sent when the SSE connection encounters an error.
type logErrorMsg struct {
	Error error
}

// LogViewer displays streaming logs from a container.
type LogViewer struct {
	cfg         *config.Config
	version     string
	serviceID   string
	deviceID    string
	containerID string
	lines       []string
	offset      int
	follow      bool
	height      int
	cancel      context.CancelFunc
	sseError    error
	spinner     components.LoadingSpinner
	connected   bool
}

// NewLogViewer creates the log viewer view.
func NewLogViewer(cfg *config.Config, version string, serviceID string, deviceID string, containerID string) *LogViewer {
	return &LogViewer{
		cfg:         cfg,
		version:     version,
		serviceID:   serviceID,
		deviceID:    deviceID,
		containerID: containerID,
		follow:      true,
		height:      20,
		lines:       []string{},
		spinner:     components.NewPulseSpinner("Connecting to log stream..."),
		connected:   false,
	}
}

func (l *LogViewer) Init() tea.Cmd {
	return tea.Batch(l.startSSE(), l.spinner.Init())
}

func (l *LogViewer) Update(msg tea.Msg) (View, tea.Cmd) {
	// Forward spinner ticks when not connected
	if !l.connected {
		var spinnerCmd tea.Cmd
		l.spinner, spinnerCmd = l.spinner.Update(msg)
		if spinnerCmd != nil {
			if _, ok := msg.(tea.KeyMsg); !ok {
				return l, spinnerCmd
			}
		}
	}

	switch msg := msg.(type) {
	case logLineMsg:
		l.connected = true
		l.spinner.Active = false
		l.lines = append(l.lines, msg.Line)
		if l.follow {
			l.offset = len(l.lines) - l.height
			if l.offset < 0 {
				l.offset = 0
			}
		}
		return l, nil

	case logErrorMsg:
		l.sseError = msg.Error
		return l, nil

	case tea.WindowSizeMsg:
		l.height = msg.Height - 6

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			l.follow = false
			if l.offset > 0 {
				l.offset--
			}
		case "down", "j":
			if l.offset < len(l.lines)-l.height {
				l.offset++
			}
		case "pgup":
			l.follow = false
			l.offset -= l.height
			if l.offset < 0 {
				l.offset = 0
			}
		case "pgdown":
			l.offset += l.height
			max := len(l.lines) - l.height
			if max < 0 {
				max = 0
			}
			if l.offset > max {
				l.offset = max
			}
		case "f":
			l.follow = !l.follow
			if l.follow {
				l.offset = len(l.lines) - l.height
				if l.offset < 0 {
					l.offset = 0
				}
			}
		case "esc":
			if l.cancel != nil {
				l.cancel()
			}
			return l, PopView()
		}
	}
	return l, nil
}

func (l *LogViewer) View() string {
	followIndicator := theme.MutedStyle.Render("FOLLOW OFF")
	if l.follow {
		followIndicator = theme.GoodStyle.Render("● FOLLOW")
	}

	title := fmt.Sprintf("%s  %s",
		lipgloss.NewStyle().Foreground(theme.Accent).Bold(true).Render("Logs — "+l.serviceID),
		followIndicator,
	)

	// Show connection error if any
	if l.sseError != nil {
		title += "\n" + theme.WarnStyle.Render("SSE Error: "+l.sseError.Error())
	}

	// Show spinner if not connected yet
	if !l.connected && len(l.lines) == 0 {
		l.lines = []string{
			l.spinner.View(),
			fmt.Sprintf("Device: %s, Container: %s", l.deviceID, l.containerID),
		}
	}

	// Visible lines
	visibleLines := l.height
	if visibleLines <= 0 {
		visibleLines = 10
	}

	start := l.offset
	end := start + visibleLines
	if end > len(l.lines) {
		end = len(l.lines)
	}
	if start > len(l.lines) {
		start = len(l.lines)
	}

	var visible []string
	for i := start; i < end; i++ {
		lineNum := theme.MutedStyle.Render(fmt.Sprintf("%4d", i+1))
		visible = append(visible, lineNum+" "+l.lines[i])
	}

	body := strings.Join(visible, "\n")

	// Scrollbar info
	scrollInfo := theme.MutedStyle.Render(fmt.Sprintf("Line %d/%d", l.offset+1, len(l.lines)))

	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(0, 1).
		Render(title + "\n" + body + "\n" + scrollInfo)

	return card
}

func (l *LogViewer) KeyBinds() []KeyBind {
	return []KeyBind{
		{Key: "↑/↓", Help: "scroll"},
		{Key: "PgUp/Dn", Help: "page"},
		{Key: "f", Help: "follow"},
		{Key: "Esc", Help: "back"},
	}
}

// startSSE starts a goroutine to read logs via SSE.
func (l *LogViewer) startSSE() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		l.cancel = cancel

		daemonAddr := l.cfg.Daemon.Listen
		if daemonAddr == "" {
			daemonAddr = "127.0.0.1:7473"
		}

		url := fmt.Sprintf("http://%s/logs/%s/%s", daemonAddr, l.deviceID, l.containerID)
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return logErrorMsg{Error: fmt.Errorf("creating request: %w", err)}
		}
		req.Header.Set("Accept", "text/event-stream")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return logErrorMsg{Error: fmt.Errorf("SSE request failed: %w", err)}
		}

		// Note: In a real implementation, we'd need a way to send messages
		// back to the UI from this goroutine. For now, we'll add some initial log lines.
		go func() {
			defer func() {
				_ = resp.Body.Close() // Best-effort close of SSE response body.
			}()
			defer cancel()

			scanner := bufio.NewScanner(resp.Body)
			for scanner.Scan() {
				line := scanner.Text()

				// Parse SSE format (lines starting with "data: ")
				if strings.HasPrefix(line, "data: ") {
					logData := line[6:] // Remove "data: " prefix
					// In a complete implementation, we'd send logData back to the UI
					// For now, this establishes the SSE connection pattern
					_ = logData
				}
			}

			if err := scanner.Err(); err != nil && ctx.Err() == nil {
				_ = err // TODO: plumb async scanner errors back into the UI message loop.
			}
		}()

		return logLineMsg{Line: fmt.Sprintf("Connected to logs for %s/%s", l.deviceID, l.containerID)}
	}
}
