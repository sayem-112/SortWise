package main

import (
	"fmt"
	"testing"
	"time"
)

func TestOnlyOneCopyStartsOnAnAddress(t *testing.T) {
	address := fmt.Sprintf("127.0.0.1:%d", time.Now().UnixNano()%50000+10000)
	release, only := claimInstance(address)
	if !only {
		t.Fatal("the first copy was turned away")
	}
	if _, again := claimInstance(address); again {
		t.Fatal("a second copy could start while the first was starting")
	}
	if _, other := claimInstance(address + "1"); !other {
		t.Fatal("a copy on another port was turned away")
	}
	release()
	next, only := claimInstance(address)
	if !only {
		t.Fatal("the address stayed claimed after the first copy let go")
	}
	next()
}
