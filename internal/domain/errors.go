package domain

import "errors"

var (
	ErrPaymentTokenNotFound = errors.New("token de pago no encontrado")
	ErrWebpayNotOffered     = errors.New("esta empresa no cobra con tarjeta")
	ErrWebpayCredential     = errors.New("no se pudo usar la credencial de Webpay de la empresa")
	ErrInvalidChargeRefs    = errors.New("uno o más cobros seleccionados no son válidos")
	ErrWebpayTxNotFound     = errors.New("transacción de pago no encontrada")
	ErrKhipuNotOffered      = errors.New("esta empresa no cobra con Khipu")
	ErrKhipuCredential      = errors.New("no se pudo usar la API key de Khipu de la empresa")
	ErrKhipuPending         = errors.New("khipu todavía no confirma la transferencia")
	ErrKhipuMismatch        = errors.New("el pago de khipu no coincide con el cobro")
	ErrFintocNotOffered     = errors.New("esta empresa no cobra con Fintoc")
	ErrFintocCredential     = errors.New("no se pudo usar la credencial de Fintoc de la empresa")
	ErrFintocMismatch       = errors.New("el pago de fintoc no coincide con el cobro")
	ErrFintocSignature      = errors.New("la firma del aviso de fintoc no es válida")
)
