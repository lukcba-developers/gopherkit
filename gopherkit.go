// Package gopherkit proporciona un conjunto de componentes empresariales para construir
// aplicaciones Go escalables y mantenibles.
//
// GopherKit incluye componentes para cache, circuit breaker, middleware, base de datos,
// CQRS, validación y más. Está diseñado siguiendo principios de Domain-Driven Design
// y patrones arquitectónicos modernos.
//
// Ejemplo de uso básico:
//
//	kit := gopherkit.New("MyApp")
//	fmt.Println(kit.Greet()) // Output: Hello from MyApp!
//	fmt.Println(kit.GetInfo()) // Output: GopherKit: MyApp (version v0.1.0)
package gopherkit

import (
	"fmt"
)

// Kit representa la estructura principal de GopherKit que encapsula
// la información básica de la aplicación.
type Kit struct {
	// Name es el nombre de la aplicación
	Name string
	// Version es la versión actual del kit
	Version string
}

// New crea una nueva instancia de Kit con el nombre especificado.
// La versión se inicializa automáticamente a "v0.1.0".
//
// Parámetros:
//   - name: El nombre de la aplicación
//
// Retorna:
//   - *Kit: Nueva instancia de Kit
//
// Ejemplo:
//
//	kit := gopherkit.New("MyApplication")
func New(name string) *Kit {
	return &Kit{
		Name:    name,
		Version: "v0.1.0",
	}
}

// Greet retorna un mensaje de saludo personalizado usando el nombre del kit.
//
// Retorna:
//   - string: Mensaje de saludo en formato "Hello from {Name}!"
//
// Ejemplo:
//
//	kit := gopherkit.New("MyApp")
//	message := kit.Greet() // "Hello from MyApp!"
func (k *Kit) Greet() string {
	return fmt.Sprintf("Hello from %s!", k.Name)
}

// GetInfo retorna información completa del kit incluyendo nombre y versión.
//
// Retorna:
//   - string: Información del kit en formato "GopherKit: {Name} (version {Version})"
//
// Ejemplo:
//
//	kit := gopherkit.New("MyApp")
//	info := kit.GetInfo() // "GopherKit: MyApp (version v0.1.0)"
func (k *Kit) GetInfo() string {
	return fmt.Sprintf("GopherKit: %s (version %s)", k.Name, k.Version)
}