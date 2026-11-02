package controller

// Tipos que solo describen respuestas en la documentación Swagger; los handlers responden con gin.H.

type errorResponse struct {
	Error string `json:"error" example:"token no encontrado"`
}

type okResponse struct {
	OK bool `json:"ok" example:"true"`
}
