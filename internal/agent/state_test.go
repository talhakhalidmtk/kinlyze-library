package agent

import "testing"

func TestAlreadyRanToday(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)        // os.UserHomeDir() on darwin/linux
	t.Setenv("USERPROFILE", tmp) // os.UserHomeDir() on windows

	if alreadyRanToday() {
		t.Fatal("expected alreadyRanToday to be false before any run is recorded")
	}

	markRanToday()

	if !alreadyRanToday() {
		t.Fatal("expected alreadyRanToday to be true right after markRanToday")
	}
}

func TestClearState(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	markRanToday()
	if !alreadyRanToday() {
		t.Fatal("expected alreadyRanToday to be true right after markRanToday")
	}

	clearState()
	if alreadyRanToday() {
		t.Fatal("expected alreadyRanToday to be false after clearState")
	}

	// clearState must be a no-op, not an error, when there's nothing to clear.
	clearState()
}
