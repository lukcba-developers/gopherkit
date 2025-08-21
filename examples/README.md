# 📚 **GopherKit Examples**

Esta carpeta contiene ejemplos completos y guías para usar GopherKit en diferentes escenarios.

## 🚀 **Ejemplos Básicos**

### [`basic/quickstart/`](./basic/quickstart/)
Ejemplo básico de "Hello World" con GopherKit.

```bash
cd examples/basic/quickstart
go run main.go
```

## 🔌 **Servicios API Completos**

### [`api-services/auth-api/`](./api-services/auth-api/)
Servicio de autenticación completo migrado de ClubPulse.
- JWT authentication
- User registration/login
- Password hashing
- Rate limiting

### [`api-services/user-api/`](./api-services/user-api/)
API de gestión de usuarios con todas las funcionalidades.
- CRUD de usuarios
- Cache Redis
- Métricas automáticas
- Health checks

## 📊 **Observabilidad**

### [`observability/metrics/`](./observability/metrics/)
Ejemplo completo de métricas con Prometheus.
- Métricas personalizadas
- Grafana dashboard
- Load testing con k6

### [`observability/otel/`](./observability/otel/)
Integración con OpenTelemetry para tracing distribuido.
- Jaeger tracing
- Métricas OTEL
- Configuración collector

## 🏗️ **Infraestructura**

### [`infrastructure/grafana/`](./infrastructure/grafana/)
Dashboards pre-configurados para GopherKit.

### [`infrastructure/prometheus/`](./infrastructure/prometheus/)
Configuración y reglas de alertas para Prometheus.

## 📖 **Guías**

### [`guides/migration-guide.md`](./guides/migration-guide.md)
Guía completa para migrar servicios existentes a GopherKit.

### Guías Futuras
- `guides/testing-guide.md` - Testing con GopherKit
- `guides/deployment-guide.md` - Despliegue en producción
- `guides/monitoring-guide.md` - Configuración de monitoreo

## 🎯 **Inicio Rápido**

1. **Ejemplo básico:**
   ```bash
   cd examples/basic/quickstart
   go run main.go
   ```

2. **Servicio completo:**
   ```bash
   cd examples/api-services/user-api
   docker-compose up -d  # Base de datos
   go run cmd/api/main.go
   ```

3. **Con observabilidad:**
   ```bash
   cd examples/observability/metrics
   docker-compose up -d  # Prometheus + Grafana
   go run main.go
   ```

## 📋 **Orden de Aprendizaje Recomendado**

1. 🟢 **Básico** → `basic/quickstart/`
2. 🟡 **API Simple** → `api-services/auth-api/`
3. 🟠 **API Completa** → `api-services/user-api/`
4. 🔴 **Observabilidad** → `observability/metrics/`
5. 🟣 **Avanzado** → `observability/otel/`

## 🤝 **Contribuir**

¿Falta un ejemplo que necesitas? ¡Abre un issue o envía un PR!

### Estructura para nuevos ejemplos:
```
examples/categoria/nombre-ejemplo/
├── README.md           # Descripción y instrucciones
├── main.go            # Código principal
├── docker-compose.yml # Dependencias (opcional)
└── .env.example       # Variables de entorno
```

---

**¿Dudas?** Revisa la [documentación principal](../docs/) o abre un issue.