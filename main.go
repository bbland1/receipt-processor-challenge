package main

// import "fmt"

func main() {
	// fmt.Println("helloooooo")
	server := NewApiServer(":3000")
	server.Run()
}