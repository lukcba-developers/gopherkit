package main

import (
	"fmt"
	"github.com/lukcba-developers/gopherkit"
)

func main() {
	kit := gopherkit.New("MyGopherApp")
	
	fmt.Println(kit.Greet())
	fmt.Println(kit.GetInfo())
}