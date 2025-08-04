/*
Package gopherkit es una librería empresarial para Go que proporciona un conjunto
de componentes reutilizables y utilidades para construir aplicaciones web escalables
y mantenibles.

# Características Principales

GopherKit está diseñado siguiendo principios de Domain-Driven Design (DDD) y
patrones arquitectónicos modernos, ofreciendo:

  - Sistema de cache unificado con Redis y memoria
  - Circuit breaker para protección contra fallos
  - Stack completo de middleware HTTP
  - Gestión de conexiones de base de datos
  - Implementación de CQRS y Event Sourcing
  - Sistema robusto de validación
  - Utilidades para testing

# Instalación

	go get github.com/lukcba-developers/gopherkit

# Uso Básico

	package main

	import (
		"fmt"
		"github.com/lukcba-developers/gopherkit"
	)

	func main() {
		kit := gopherkit.New("MyApp")
		fmt.Println(kit.Greet())   // Hello from MyApp!
		fmt.Println(kit.GetInfo()) // GopherKit: MyApp (version v0.1.0)
	}

# Paquetes Disponibles

La librería está organizada en múltiples paquetes especializados:

## Cache (pkg/cache)

Sistema de cache híbrido con soporte para Redis y memoria local:

	import "github.com/lukcba-developers/gopherkit/pkg/cache"

	config := cache.DefaultCacheConfig()
	cache, err := cache.NewUnifiedCache(config, logger)

## Circuit Breaker (pkg/circuitbreaker)

Patrón Circuit Breaker para protección contra fallos en cascada:

	import "github.com/lukcba-developers/gopherkit/pkg/circuitbreaker"

	config := circuitbreaker.DefaultCircuitBreakerConfig()
	cb, err := circuitbreaker.New(config)

## Middleware (pkg/middleware)

Stack completo de middleware para APIs HTTP:

	import "github.com/lukcba-developers/gopherkit/pkg/middleware"

	stack := middleware.ProductionMiddlewareStack(logger, "MyAPI")
	stack.ApplyMiddlewares(router)

## Base de Datos (pkg/database)

Gestión de conexiones PostgreSQL con múltiples ORMs:

	import "github.com/lukcba-developers/gopherkit/pkg/database/connection"

	config := connection.DefaultPostgresConfig()
	manager := connection.NewPostgresConnectionManager(config, logger)

## CQRS (pkg/infrastructure/cqrs)

Implementación de Command Query Responsibility Segregation:

	import "github.com/lukcba-developers/gopherkit/pkg/infrastructure/cqrs"

	commandBus := cqrs.NewCommandBus(logger)
	queryBus := cqrs.NewQueryBus(logger)

# Documentación Adicional

Para más información detallada, consulta:

  - API Reference: docs/API.md
  - Architecture Guide: docs/ARCHITECTURE.md
  - Examples: docs/EXAMPLES.md

# Contribuir

Las contribuciones son bienvenidas. Por favor consulta las guías de contribución
en el repositorio de GitHub.

# Licencia

Este proyecto está licenciado bajo la Licencia MIT.
*/
package gopherkit