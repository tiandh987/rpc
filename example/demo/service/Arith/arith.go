package Arith

import "log"

type Arith struct {}

type ArithRequest struct {
	A int
	B int
}

type ArithResponse struct {
	Pro int // 积
	Quo int // 商
	Rem int // 余数
}

func (a *Arith) Mul(req ArithRequest, resp *ArithResponse) error {
	log.Printf("Mul req.A: %d, req.B: %d\n", req.A, req.B)

	resp.Pro = req.A * req.B
	return nil
}

