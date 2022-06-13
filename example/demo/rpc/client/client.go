package main

import (
	"github.com/tiandh987/rpc/example/demo/service/Arith"
	"log"
	"net/rpc"
)

func main() {
	client, err := rpc.Dial("tcp", "localhost:1235")
	if err != nil {
		log.Fatal(err)
	}

	req := Arith.ArithRequest{
		A: 9,
		B: 2,
	}

	var resp Arith.ArithResponse

	err = client.Call("Arith.Mul", req, &resp)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Arith.Mul req.A : %d, req.B : %d, resp.Pro : %d\n", req.A, req.B, resp.Pro)
}
