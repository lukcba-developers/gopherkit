# 🚀 **GopherKit - Ejemplo Básico**

Este es el ejemplo más simple para empezar con GopherKit.

## 📋 **Requisitos**

- Go 1.21+
- GopherKit instalado

## 🔧 **Instalación**

```bash
# Instalar GopherKit
go get github.com/lukcba-developers/gopherkit

# Ejecutar el ejemplo
go run main.go
```

## 📝 **Qué hace este ejemplo**

1. Inicializa GopherKit con el nombre "MyGopherApp"
2. Muestra el saludo básico
3. Muestra información del kit

## 🔍 **Código Explicado**

```go
package main

import (
    "fmt"
    "github.com/lukcba-developers/gopherkit"
)

func main() {
    // Crear una nueva instancia de GopherKit
    kit := gopherkit.New("MyGopherApp")
    
    // Mostrar saludo
    fmt.Println(kit.Greet())
    
    // Mostrar información
    fmt.Println(kit.GetInfo())
}
```

## ➡️ **Siguiente Paso**

Una vez que este ejemplo funcione, prueba:
- [`../api-services/auth-api/`](../../api-services/auth-api/) - Servicio completo
- [`../observability/metrics/`](../../observability/metrics/) - Con métricas

## 🤝 **¿Problemas?**

Abre un issue en el [repositorio principal](https://github.com/lukcba-developers/gopherkit/issues).