package main

import (
	"fmt"
	"reflect"
	"github.com/sandertv/gophertunnel/minecraft"
)

func main() {
	t := reflect.TypeOf(minecraft.GameData{})
	fmt.Println("GameData fields:")
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fmt.Printf("  %s %s\n", f.Name, f.Type)
	}
}
