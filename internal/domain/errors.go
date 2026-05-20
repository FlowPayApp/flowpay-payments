package domain

import "errors"

var (
	ErrPaymentTokenNotFound = errors.New("token de pago no encontrado")
	ErrWebpayNotConfigured  = errors.New("webpay no está configurado en el servidor")
	ErrInvalidChargeRefs    = errors.New("uno o más cobros seleccionados no son válidos")
	ErrWebpayTxNotFound     = errors.New("transacción de pago no encontrada")
)
