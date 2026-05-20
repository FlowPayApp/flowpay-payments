# flowpay-payments

Microservicio de **pagos** del ecosistema FlowPay. Aquí vive todo lo relacionado con cobrar dinero: portal público para que el cliente pague, pasarela **Transbank Webpay Plus**, registro de pagos y tokens de acceso al portal.

## De qué va este repositorio

FlowPay separa responsabilidades en varios servicios:

| Repositorio | Responsabilidad |
|-------------|-----------------|
| `flowpay-backend` | Cobros (charges), clientes, recordatorios, dashboard, WhatsApp |
| `flowpay-sso` | Login y emisión de JWT |
| **`flowpay-payments`** | **Liquidación de pagos** (manual, transferencia informada en portal, tarjeta vía Webpay) |
| `flowpay-frontend` | Interfaz web |

Este repo **no** crea cobros ni envía recordatorios. Trabaja sobre cobros que ya existen en la base de datos: muestra lo pendiente al deudor, inicia el pago con Transbank y, al confirmarse, marca los cobros como pagados y escribe en la tabla `payments`.

### Qué hace en la práctica

1. **Portal público** (`/pay/:token`) — El cliente abre un enlace sin login y ve sus cobros pendientes, vencidos y pagados, más instrucciones de transferencia si la empresa las configuró.
2. **Tokens de pago** — El equipo interno (con JWT) genera un token opaco por cliente para compartir ese enlace.
3. **Webpay Plus** — Checkout con tarjeta: crea transacción en BD, redirige a Transbank, recibe el `return_url` y confirma el pago en servidor (commit).
4. **Pago manual** — Registro de un pago hecho fuera de la pasarela (efectivo, transferencia ya recibida, etc.) sobre un `charge_id`.

### Qué no hace (sigue en el backend)

- CRUD de cobros y clientes  
- Recordatorios por correo/WhatsApp  
- Dashboard y métricas de cobranza  
- Adjuntos de cobro (el portal puede mostrar el token del adjunto; el archivo lo sirve el backend)

En desarrollo comparte **la misma PostgreSQL** que `flowpay-backend` (Supabase o local). Las tablas relevantes son `payment_tokens`, `payment_transactions`, `payment_transaction_charges`, `payments` y `charges`.

## Stack

- Go 1.24+, Gin, `database/sql` + pgx  
- JWT compatible con `flowpay-sso`  
- Integración HTTP con Transbank (sin SDK oficial)

## Arranque local

```bash
cp .env.example .env
go run ./cmd/server
```

- Puerto por defecto: **`:8081`** (`flowpay-backend` usa `:8080`)  
- Health: `GET http://localhost:8081/health`  
- Para Webpay en local hace falta una URL pública HTTPS (ngrok) apuntando a **8081** en `FLOWPAY_PAYMENTS_PUBLIC_BASE_URL`

## API principal

| Método | Ruta | Auth | Descripción |
|--------|------|------|-------------|
| GET | `/health` | — | Estado del servicio |
| GET | `/api/public/pay/:token` | — | Datos del portal de pago |
| POST | `/api/public/pay/:token/checkout` | — | Iniciar pago Webpay |
| POST | `/api/public/pay/:token/commit` | — | Confirmar pago (JSON) |
| GET/POST | `/api/public/webpay/return/:token` | — | Callback de Transbank |
| GET | `/api/public/webpay/bridge/:id` | — | Redirección POST a Webpay |
| POST | `/api/payment-tokens` | JWT | Crear token de portal |
| POST | `/api/payments` | JWT | Registrar pago manual |

## Variables de entorno

Prefijo `FLOWPAY_PAYMENTS_*`. Si no están definidas, se usa fallback a `FLOWPAY_*` (misma BD, JWT o Transbank que el backend).

Copia `.env.example` y configura al menos: DSN, JWT (igual que SSO/backend), y para Webpay la URL pública y credenciales Transbank.

## Estructura del código

```
flowpay-payments/
├── cmd/server/           → main
├── internal/
│   ├── routes/           → URLs
│   ├── controller/       → HTTP (entrada/salida)
│   ├── service/          → lógica de negocio
│   ├── repository/       → SQL
│   ├── model/            → entidades de BD
│   ├── gateway/transbank → cliente API Transbank
│   └── ...
```

Flujo de una petición: **ruta → controlador → servicio → repositorio → PostgreSQL**.

## Esquema y migraciones

Scripts SQL en `../Mysql/postgresql_migration/` (compartidos con el monolito).

## Integración con el resto

1. **Frontend**: las llamadas de pago deben ir a la URL de este servicio (ej. `http://localhost:8081`), no al backend.  
2. **Ngrok / producción**: `FLOWPAY_PAYMENTS_PUBLIC_BASE_URL` debe ser la URL de **payments**, no la del backend.  
3. **Backend**: cuando el front ya use payments, se pueden eliminar las rutas duplicadas de pagos en `flowpay-backend`.
