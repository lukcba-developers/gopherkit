# Comandos Personalizados para Claude Code

Este archivo documenta todos los comandos slash disponibles en `.claude/commands/` para usarlos directamente desde la terminal de Claude Code. Cada comando está optimizado para flujos de trabajo profesionales, funciona con cualquier lenguaje de programación y está en español.

---

## Índice
- [analisis](#analisis)
- [funcionalidad](#funcionalidad)
- [calidad](#calidad)
- [fix-error](#fix-error)
- [review](#review)
- [explain](#explain)
- [test](#test)
- [optimize](#optimize)
- [debug](#debug)
- [refactor](#refactor)
- [security](#security)
- [docs](#docs)
- [convert](#convert)

---

## `/analisis`
**Descripción:** Análisis exhaustivo del código fuente del proyecto.

**Ejemplo de uso:**
```
/analisis Analiza la estructura y calidad del proyecto actual
```

---

## `/funcionalidad`
**Descripción:** Implementa una nueva funcionalidad siguiendo las mejores prácticas.

**Ejemplo de uso:**
```
/funcionalidad Implementar login con Google OAuth
```

---

## `/calidad`
**Descripción:** Revisión exhaustiva de la calidad del código.

**Ejemplo de uso:**
```
/calidad Revisa la calidad del código en el directorio actual
```

---

## `/fix-error`
**Descripción:** Analiza y resuelve errores de código.

**Ejemplo de uso:**
```
/fix-error panic: runtime error: index out of range
```

---

## `/review`
**Descripción:** Revisión profesional de código.

**Ejemplo de uso:**
```
/review func suma(a, b int) int { return a + b }
```

---

## `/explain`
**Descripción:** Explicación didáctica de código.

**Ejemplo de uso:**
```
/explain for i := 0; i < 10; i++ { fmt.Println(i) }
```

---

## `/test`
**Descripción:** Genera tests unitarios y edge cases.

**Ejemplo de uso:**
```
/test func suma(a, b int) int { return a + b }
```

---

## `/optimize`
**Descripción:** Optimiza el rendimiento y la eficiencia de código.

**Ejemplo de uso:**
```
/optimize for i := 0; i < len(arr); i++ { ... }
```

---

## `/debug`
**Descripción:** Debugging interactivo de problemas.

**Ejemplo de uso:**
```
/debug No se conecta a la base de datos
```

---

## `/refactor`
**Descripción:** Refactorización avanzada de código.

**Ejemplo de uso:**
```
/refactor func suma(a, b int) int { return a + b }
```

---

## `/security`
**Descripción:** Análisis de seguridad en código.

**Ejemplo de uso:**
```
/security db.Query("SELECT * FROM users WHERE id = " + userID)
```

---

## `/docs`
**Descripción:** Generación de documentación.

**Ejemplo de uso:**
```
/docs func suma(a, b int) int { return a + b }
```

---

## `/convert`
**Descripción:** Conversión de código entre lenguajes de programación.

**Ejemplo de uso:**
```
/convert def suma(a, b): return a + b de python a go
```

---

> **Nota:** Todos los comandos detectan automáticamente el lenguaje de programación y adaptan sus recomendaciones y herramientas sugeridas según el stack detectado. Puedes personalizar y ampliar estos comandos creando nuevos archivos `.md` en `.claude/commands/` siguiendo el mismo formato. 