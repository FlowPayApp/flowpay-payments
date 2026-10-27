package service

import "testing"

func TestTransferOfferedKeepsBoth(t *testing.T) {
	if !transferOffered(true, true, khipuGateway) || !transferOffered(true, true, fintocGateway) {
		t.Fatal("con las dos credenciales se ofrecen las dos")
	}
	if !transferOffered(true, false, khipuGateway) || transferOffered(true, false, fintocGateway) {
		t.Fatal("solo khipu")
	}
	if transferOffered(false, true, khipuGateway) || !transferOffered(false, true, fintocGateway) {
		t.Fatal("solo fintoc")
	}
	if transferOffered(false, false, khipuGateway) || transferOffered(false, false, fintocGateway) {
		t.Fatal("sin credencial")
	}
}
