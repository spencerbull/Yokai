package views

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Mock view for testing
type mockView struct {
	initCalled   bool
	updateCalled bool
	viewCalled   bool
}

func (m *mockView) Init() tea.Cmd {
	m.initCalled = true
	return nil
}

func (m *mockView) Update(msg tea.Msg) (View, tea.Cmd) {
	m.updateCalled = true
	return m, nil
}

func (m *mockView) View() string {
	m.viewCalled = true
	return "mock view"
}

func (m *mockView) KeyBinds() []KeyBind {
	return []KeyBind{
		{Key: "q", Help: "quit"},
		{Key: "enter", Help: "select"},
	}
}

func TestNavigateCmd(t *testing.T) {
	t.Parallel()

	targetView := &mockView{}

	// Get the command
	cmd := Navigate(targetView)
	if cmd == nil {
		t.Fatal("Navigate should return a non-nil command")
	}

	// Execute the command to get the message
	msg := cmd()

	// Check that it returns a NavigateMsg
	navigateMsg, ok := msg.(NavigateMsg)
	if !ok {
		t.Fatalf("expected NavigateMsg, got %T", msg)
	}

	// Check the message contents
	if navigateMsg.Target != targetView {
		t.Error("NavigateMsg target should match the target view")
	}

	if navigateMsg.Replace {
		t.Error("Navigate should not set Replace to true")
	}
}

func TestNavigateReplaceCmd(t *testing.T) {
	t.Parallel()

	targetView := &mockView{}

	// Get the command
	cmd := NavigateReplace(targetView)
	if cmd == nil {
		t.Fatal("NavigateReplace should return a non-nil command")
	}

	// Execute the command to get the message
	msg := cmd()

	// Check that it returns a NavigateMsg
	navigateMsg, ok := msg.(NavigateMsg)
	if !ok {
		t.Fatalf("expected NavigateMsg, got %T", msg)
	}

	// Check the message contents
	if navigateMsg.Target != targetView {
		t.Error("NavigateMsg target should match the target view")
	}

	if !navigateMsg.Replace {
		t.Error("NavigateReplace should set Replace to true")
	}
}

func TestPopViewCmd(t *testing.T) {
	t.Parallel()

	// Get the command
	cmd := PopView()
	if cmd == nil {
		t.Fatal("PopView should return a non-nil command")
	}

	// Execute the command to get the message
	msg := cmd()

	// Check that it returns a PopViewMsg
	_, ok := msg.(PopViewMsg)
	if !ok {
		t.Fatalf("expected PopViewMsg, got %T", msg)
	}
}

func TestNavigateMsgStructure(t *testing.T) {
	t.Parallel()

	targetView := &mockView{}

	// Test NavigateMsg construction
	msg := NavigateMsg{
		Target:  targetView,
		Replace: true,
	}

	if msg.Target != targetView {
		t.Error("NavigateMsg Target field should be accessible")
	}
	if !msg.Replace {
		t.Error("NavigateMsg Replace field should be accessible")
	}

	// Test zero value
	var zeroMsg NavigateMsg
	if zeroMsg.Target != nil {
		t.Error("zero value NavigateMsg should have nil Target")
	}
	if zeroMsg.Replace {
		t.Error("zero value NavigateMsg should have Replace false")
	}
}

func TestPopViewMsgStructure(t *testing.T) {
	t.Parallel()

	// PopViewMsg is an empty struct, test that it can be created
	var popMsg PopViewMsg

	// Should be able to use it in type assertions
	var msg tea.Msg = popMsg
	if _, ok := msg.(PopViewMsg); !ok {
		t.Error("PopViewMsg should satisfy tea.Msg interface")
	}
}

func TestKeyBindStructure(t *testing.T) {
	t.Parallel()

	keyBind := KeyBind{
		Key:  "enter",
		Help: "select item",
	}

	if keyBind.Key != "enter" {
		t.Errorf("expected Key 'enter', got %s", keyBind.Key)
	}
	if keyBind.Help != "select item" {
		t.Errorf("expected Help 'select item', got %s", keyBind.Help)
	}

	// Test zero value
	var zeroKeyBind KeyBind
	if zeroKeyBind.Key != "" {
		t.Error("zero value KeyBind should have empty Key")
	}
	if zeroKeyBind.Help != "" {
		t.Error("zero value KeyBind should have empty Help")
	}
}

func TestViewInterface(t *testing.T) {
	t.Parallel()

	// Test that mockView implements View interface
	var view View = &mockView{}

	// Test Init
	cmd := view.Init()
	mockV := view.(*mockView)
	if !mockV.initCalled {
		t.Error("Init should have been called")
	}

	// cmd can be nil, which is fine
	_ = cmd

	// Test Update
	_, updateCmd := view.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !mockV.updateCalled {
		t.Error("Update should have been called")
	}

	// updateCmd can be nil, which is fine
	_ = updateCmd

	// Test View
	viewStr := view.View()
	if !mockV.viewCalled {
		t.Error("View should have been called")
	}
	if viewStr != "mock view" {
		t.Errorf("expected view string 'mock view', got %s", viewStr)
	}

	// Test KeyBinds
	keyBinds := view.KeyBinds()
	if len(keyBinds) != 2 {
		t.Errorf("expected 2 keybinds, got %d", len(keyBinds))
	}

	if keyBinds[0].Key != "q" {
		t.Errorf("expected first keybind key 'q', got %s", keyBinds[0].Key)
	}
	if keyBinds[0].Help != "quit" {
		t.Errorf("expected first keybind help 'quit', got %s", keyBinds[0].Help)
	}
}

func TestCommandsReturnFunctions(t *testing.T) {
	t.Parallel()

	targetView := &mockView{}

	// Test that Navigate returns a function
	navigateCmd := Navigate(targetView)
	if navigateCmd == nil {
		t.Error("Navigate should return a function")
	}

	// Test that the function returns a message when called
	navigateMsg := navigateCmd()
	if navigateMsg == nil {
		t.Error("Navigate command should return a message")
	}

	// Test NavigateReplace
	navigateReplaceCmd := NavigateReplace(targetView)
	if navigateReplaceCmd == nil {
		t.Error("NavigateReplace should return a function")
	}

	navigateReplaceMsg := navigateReplaceCmd()
	if navigateReplaceMsg == nil {
		t.Error("NavigateReplace command should return a message")
	}

	// Test PopView
	popCmd := PopView()
	if popCmd == nil {
		t.Error("PopView should return a function")
	}

	popMsg := popCmd()
	if popMsg == nil {
		t.Error("PopView command should return a message")
	}
}

func TestMessageTypes(t *testing.T) {
	t.Parallel()

	// Test that messages implement tea.Msg
	targetView := &mockView{}

	var msg tea.Msg

	// NavigateMsg should implement tea.Msg
	msg = NavigateMsg{Target: targetView, Replace: false}
	_ = msg // Use the variable to avoid unused error

	// PopViewMsg should implement tea.Msg
	msg = PopViewMsg{}
	_ = msg // Use the variable to avoid unused error

	// This test passes if compilation succeeds
}

func TestNavigateMessageComparison(t *testing.T) {
	t.Parallel()

	view1 := &mockView{}
	view2 := &mockView{}

	// Test different navigate messages
	msg1 := NavigateMsg{Target: view1, Replace: false}
	msg2 := NavigateMsg{Target: view1, Replace: false}
	msg3 := NavigateMsg{Target: view2, Replace: false}
	msg4 := NavigateMsg{Target: view1, Replace: true}

	// Same view, same replace flag - should be considered equal in structure
	if msg1.Target != msg2.Target || msg1.Replace != msg2.Replace {
		t.Error("messages with same target and replace should have same field values")
	}

	// Different view - targets should be different
	if msg1.Target == msg3.Target {
		t.Error("messages with different targets should have different Target field")
	}

	// Same view, different replace flag
	if msg1.Replace == msg4.Replace {
		t.Error("messages with different Replace flags should have different Replace field")
	}
}
