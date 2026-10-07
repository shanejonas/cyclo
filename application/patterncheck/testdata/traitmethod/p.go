package main

import "fmt"

// Parallel types whose Speak methods differ only in the helper they call.
// The miner should propose a trait_method candidate for the sound method.
type Dog struct{ name string }

func (d Dog) Speak() string {
	s := d.sound()
	return fmt.Sprintf("%s says %s", d.name, s)
}

func (d Dog) sound() string { return "woof" }

type Cat struct{ name string }

func (c Cat) Speak() string {
	s := c.sound()
	return fmt.Sprintf("%s says %s", c.name, s)
}

func (c Cat) sound() string { return "meow" }

func main() {
	fmt.Println(Dog{name: "rex"}.Speak())
	fmt.Println(Cat{name: "tom"}.Speak())
}
