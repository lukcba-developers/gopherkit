package gopherkit

import (
	"testing"
)

func TestNew(t *testing.T) {
	kit := New("TestKit")
	
	if kit == nil {
		t.Fatal("Expected kit to be non-nil")
	}
	
	if kit.Name != "TestKit" {
		t.Errorf("Expected name to be 'TestKit', got '%s'", kit.Name)
	}
	
	if kit.Version != "v0.1.0" {
		t.Errorf("Expected version to be 'v0.1.0', got '%s'", kit.Version)
	}
}

func TestGreet(t *testing.T) {
	kit := New("TestKit")
	greeting := kit.Greet()
	
	expected := "Hello from TestKit!"
	if greeting != expected {
		t.Errorf("Expected '%s', got '%s'", expected, greeting)
	}
}

func TestGetInfo(t *testing.T) {
	kit := New("TestKit")
	info := kit.GetInfo()
	
	expected := "GopherKit: TestKit (version v0.1.0)"
	if info != expected {
		t.Errorf("Expected '%s', got '%s'", expected, info)
	}
}