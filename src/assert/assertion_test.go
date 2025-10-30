package assert

import (
	"errors"
	"testing"
)

func TestAssertT_PassingCondition(t *testing.T) {
	// This should not fail
	AssertT(t, true, "This should pass")
}

func TestAssertT_WithNoMessages(t *testing.T) {
	// This should not fail
	AssertT(t, true)
}

func TestAssertT_WithMultipleMessages(t *testing.T) {
	// This should not fail
	AssertT(t, true, "message 1", "message 2", 42)
}

func TestTryStringify_Error(t *testing.T) {
	err := errors.New("test error")
	result := tryStringify(err)

	if result != err {
		t.Errorf("tryStringify should return error as-is, got %v", result)
	}
}

func TestTryStringify_Stringer(t *testing.T) {
	s := &testStringer{value: "test"}
	result := tryStringify(s)

	if result != "test" {
		t.Errorf("tryStringify should call String(), got %v", result)
	}
}

func TestTryStringify_Other(t *testing.T) {
	value := 42
	result := tryStringify(value)

	if result != value {
		t.Errorf("tryStringify should return value as-is, got %v", result)
	}
}

// Test stringer implementation
type testStringer struct {
	value string
}

func (t *testStringer) String() string {
	return t.value
}
