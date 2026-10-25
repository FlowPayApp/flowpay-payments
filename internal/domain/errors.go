package domain

import "errors"

var (
	ErrPaymentTokenNotFound = errors.New("token de pago no encontrado")
	ErrWebpayNotOffered     = errors.New("esta empresa no cobra con tarjeta")
	ErrWebpayCredential     = errors.New("no se pudo usar la credencial de Webpay de la empresa")
	ErrInvalidChargeRefs    = errors.New("uno o más cobros seleccionados no son válidos")
	ErrWebpayTxNotFound     = errors.New("transacción de pago no encontrada")
)
