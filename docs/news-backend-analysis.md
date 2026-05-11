# Análisis técnico del backend de noticias

## Estado de implementación posterior

Se ejecutaron las mejoras de mayor impacto:

- Se agregaron constantes y validaciones centralizadas para `status`, `importance` y `category`.
- Se corrigieron strings con encoding roto en código Go.
- Se integró auditoría de sincronizaciones usando `job_runs`.
- `InsertFromReel` ahora reporta si realmente insertó o si PostgreSQL ignoró el insert por conflicto.
- Se agregaron endpoints admin para editar, publicar y ocultar noticias.
- Se agregaron endpoints para consultar ejecuciones del worker.
- Se documentó la estrategia recomendada para ejecutar el worker automáticamente en `docs/news-worker-scheduling.md`.
- Se agregó scheduler interno configurable por `NEWS_SYNC_INTERVAL_MINUTES`.
- Se agregó filtro por ventana reciente usando `NEWS_RECENT_DAYS`.
- Se agregó descarte de contenido que no alcanza señales mínimas de noticia.
- Instagram Stories quedan explícitamente fuera del soporte automático porque el proyecto no debe depender de proveedores externos y el HTML público no es confiable para Stories.

## Resumen ejecutivo

El backend ya tiene una base funcional para exponer, crear y sincronizar noticias desde Instagram Reels, pero todavía no tiene un mecanismo automático de ejecución del worker dentro del propio sistema. Hoy existen dos formas de disparar la sincronización:

1. Ejecutar manualmente el binario `cmd/worker`.
2. Llamar el endpoint protegido `POST /api/admin/news/sync`.

Eso significa algo importante: **el backend no agenda el worker solo**. Si nadie ejecuta el binario o pega al endpoint, no se agregan nuevas noticias automáticamente. Acá no hay magia, hermano: tener un worker compilable no es lo mismo que tener un scheduler productivo.

## Estructura actual del proyecto

```txt
cmd/
  api/
    main.go
  worker/
    main.go

internal/
  config/
    config.go
  modules/
    news/
      classifier.go
      extractor.go
      handler.go
      instagram_html_extractor.go
      model.go
      repository.go
      service.go
      syncer.go
  platform/
    database/
      postgres.go
    httpserver/
      router.go

migrations/
  00001_create_news_items.sql

docker-compose.yml
go.mod
```

La separación general está razonable para un backend chico:

- `cmd/api`: punto de entrada HTTP.
- `cmd/worker`: punto de entrada para ejecutar sincronización batch.
- `internal/modules/news`: módulo de dominio/aplicación de noticias.
- `internal/platform`: infraestructura compartida.
- `migrations`: esquema inicial de base de datos.

## Módulo `news`

### `model.go`

Define los modelos usados por API, sincronización y administración:

- `NewsItem`: modelo público para listar/ver noticias publicadas.
- `ListParams`, `ListResponse`, `Pagination`: contrato de listado.
- `ExtractedReel`: dato crudo extraído desde Instagram.
- `NewsClassification`: resultado de clasificación.
- `SyncStats`: métricas del proceso de sincronización.
- `CreateManualNewsRequest`: payload para crear noticias manuales.
- `AdminNewsItem`: vista administrativa.

Bien: hay modelos claros para lectura pública, admin y sync.

Falta mejorar:

- Separar DTOs HTTP de entidades internas si el módulo crece.
- Usar tipos/enums para `status`, `importance` y `category`, en vez de strings sueltos.
- Corregir problemas de encoding visibles previamente en strings como `Académico`, `inválido`, `Tecnología`. Eso es una señal de archivo guardado con codificación incorrecta o texto pegado mal.

### `handler.go`

Expone estas rutas bajo `/api`:

```txt
GET  /api/news
GET  /api/news/{id}
GET  /api/admin/news
POST /api/admin/news
POST /api/admin/news/sync
```

Las rutas públicas solo devuelven noticias con `status = 'published'`.

Las rutas admin usan autorización simple:

```txt
Authorization: Bearer <NEWS_SYNC_SECRET>
```

Bien:

- Hay separación entre público y admin.
- La creación manual valida `title`, `summary`, `status` e `importance`.
- El sync manual está protegido.

Falta:

- Validar `category`.
- Validar formato de `imageUrl` y `reelUrl`.
- Evitar `strings.EqualFold` para comparar bearer tokens. Para secretos conviene comparación constante (`subtle.ConstantTimeCompare`) para reducir riesgo de timing leaks.
- Diferenciar errores de DB, validación y no encontrado.
- Agregar middleware/auth real si esto va a producción.

### `service.go`

Actualmente es una capa muy delgada sobre el repositorio:

- `List`
- `GetByID`
- `CreateManual`
- `ListAdmin`

Bien para arrancar, pero hoy casi no tiene reglas de negocio. Varias decisiones viven en `repository.go`, por ejemplo defaults de categoría, importancia, body, reel manual y status.

Recomendación: mover reglas de negocio al service y dejar el repository como persistencia pura. Es como construir una casa: el repositorio debería ser la cuadrilla que pone ladrillos, no el arquitecto que decide dónde va la cocina.

### `repository.go`

Responsable de SQL contra PostgreSQL.

Hace:

- Listado público con filtros por categoría/importancia.
- Consulta por ID solo de noticias publicadas.
- Detección de duplicados por `(source, source_media_id)`.
- Inserción desde Reel.
- Inserción manual.
- Listado administrativo.

Fortalezas:

- Usa parámetros SQL, no concatena valores directamente.
- Tiene `ON CONFLICT (source, source_media_id) DO NOTHING`.
- El esquema tiene índices útiles para estado, categoría y unicidad.
- El listado público pagina con `limit` y `offset`.

Riesgos/faltantes:

- `InsertFromReel` no devuelve si realmente insertó o si el `ON CONFLICT` ignoró la fila. El sync incrementa `Inserted` si `Exec` no falla, aunque en una carrera podría no haberse insertado nada.
- La validación de duplicados ocurre antes del insert, pero la garantía real está en el índice único. Bien por el índice, pero el contador puede quedar inexacto.
- No hay transacciones porque por ahora cada item es independiente. Está bien, pero si se agregan tablas relacionadas va a hacer falta.
- `updated_at` no se actualiza automáticamente.
- `job_runs` existe en migración, pero no se usa en el código.

### `extractor.go`

Define la interfaz:

```go
type ReelExtractor interface {
    GetLatestReels(ctx context.Context, username string) ([]ExtractedReel, error)
}
```

Esto está bien diseñado: permite cambiar el extractor sin tocar el syncer. Por ejemplo, mañana se puede reemplazar HTML scraping por API oficial o por otro mecanismo propio.

### `instagram_html_extractor.go`

Implementa extracción HTML desde Instagram:

- Normaliza el username.
- Consulta:
  - `https://www.instagram.com/{username}/reels/`
  - `https://www.instagram.com/{username}/`
- Busca shortcodes de Reels con regex.
- Deduplica shortcodes en memoria.
- Limita a `maxReels`.
- Consulta cada Reel individual para obtener metadata:
  - `og:description`
  - `og:title`
  - `og:image`

Bien:

- Tiene timeout HTTP.
- Deduplica shortcodes antes de procesar.
- Sanitiza usernames.
- Usa interfaz, así que es reemplazable.

Problemas importantes:

- Instagram puede bloquear, cambiar HTML o devolver contenido incompleto. Este extractor es frágil por naturaleza.
- No hay retry/backoff.
- No hay métricas persistidas de fallos.
- `PublishedAt` queda siempre `nil`.
- Usa `User-Agent: CBTICBackend/1.0`, lo que puede aumentar bloqueo.
- Los logs son útiles para debugging, pero pueden ser ruidosos en producción.

### `classifier.go`

Clasificador simple por palabras clave:

- Categoriza según keywords.
- Calcula score.
- Convierte score en confidence.
- Genera título y resumen desde caption.

Bien:

- Es simple, entendible y determinístico.
- No depende de servicios externos.
- Permite auto-publicar según umbral.

Limitaciones:

- No entiende contexto.
- Puede clasificar mal captions cortos o con emojis.
- No detecta idioma ni intención.
- No valida si el Reel realmente es una noticia.
- Hay variables de entorno `OPENAI_API_KEY` y `OPENAI_MODEL`, pero no se usan.
- Hay variables de OpenAI en config, pero no se usan en implementación.

### `syncer.go`

Es el corazón del proceso automático/manual de ingesta.

Flujo actual:

```txt
1. Validar INSTAGRAM_TARGET_USERNAME.
2. Extraer últimos Reels.
3. Por cada Reel:
   3.1 Verificar si ya existe por source + source_media_id.
   3.2 Si existe y está en needs_review, publicarlo y contar duplicate.
   3.3 Si no existe, clasificar.
   3.4 Si sirve como noticia, definir status published.
   3.5 Insertar en news_items.
4. Devolver SyncStats.
```

La validación de “nuevas noticias” se hace así:

- Primero en aplicación: `ExistsBySourceMediaID(ctx, "instagram", reel.SourceMediaID)`.
- Después en base de datos: índice único `(source, source_media_id)` y `ON CONFLICT DO NOTHING`.

Eso está conceptualmente bien: **validación optimista en código + garantía fuerte en DB**.

Lo que falta es ajustar métricas y auditoría:

- Saber si `ON CONFLICT DO NOTHING` insertó o no insertó.
- Registrar cada corrida en `job_runs`.
- Guardar errores por item para diagnóstico.
- Tener un endpoint o tabla para ver última sincronización.

## Worker actual

### `cmd/worker/main.go`

El worker:

1. Carga `.env`.
2. Carga config.
3. Conecta a PostgreSQL.
4. Construye repository, extractor, classifier y syncer.
5. Ejecuta `syncer.Sync(ctx)`.
6. Loguea stats.
7. Termina el proceso.

Esto es un **worker one-shot**, no un daemon.

O sea:

```txt
go run ./cmd/worker
```

ejecuta una sincronización y finaliza.

Antes no había:

- Loop interno.
- Cron interno.
- Scheduler.
- Cola.
- Docker service para worker.
- Kubernetes CronJob.
- Registro en `job_runs`.

Ahora el API puede ejecutar sincronizaciones periódicas con:

```env
NEWS_SYNC_INTERVAL_MINUTES=60
NEWS_SYNC_ON_STARTUP=false
```

El binario `cmd/worker` sigue siendo one-shot y sirve para cron externo o jobs de plataforma.

## API actual

### Pública

#### `GET /api/news`

Lista noticias publicadas.

Query params:

- `category`
- `importance`
- `limit`
- `offset`

#### `GET /api/news/{id}`

Obtiene una noticia publicada por ID.

### Admin

Todas requieren:

```txt
Authorization: Bearer <NEWS_SYNC_SECRET>
```

#### `GET /api/admin/news`

Lista todas las noticias, incluyendo drafts, hidden y needs_review.

#### `POST /api/admin/news`

Crea una noticia manual.

Campos relevantes:

- `title`: obligatorio.
- `summary`: obligatorio.
- `body`: opcional.
- `imageUrl`: opcional.
- `reelUrl`: opcional.
- `category`: opcional, default `Institucional`.
- `importance`: opcional, default `media`.
- `status`: opcional, default `published`.

#### `POST /api/admin/news/sync`

Ejecuta sincronización de Instagram en el request HTTP.

Esto sirve para administración, pero no es ideal para procesos largos porque:

- Depende del timeout del cliente/proxy.
- Bloquea la request hasta terminar.
- No persiste estado de job.
- No permite reintentos controlados.

## Base de datos

### `news_items`

Tabla principal.

Campos importantes:

- `source`: origen (`instagram`, `manual`).
- `source_media_id`: ID estable del origen.
- `reel_url`: URL del reel o URL manual.
- `thumbnail_url`.
- `caption`.
- `title`, `summary`, `body`.
- `category`.
- `importance`.
- `confidence`.
- `status`.
- `published_at`.
- `scraped_at`.
- `classified_at`.
- `created_at`, `updated_at`.

Índices:

- Unique `(source, source_media_id)`.
- Unique `reel_url`.
- `(status, published_at DESC)`.
- `category`.

### `job_runs`

Existe en migración, pero no se usa.

Esto es una oportunidad clara: si querés worker serio, esta tabla debería registrar:

- Inicio.
- Fin.
- Estado.
- Cantidad extraída.
- Nuevas.
- Duplicadas.
- Insertadas.
- Fallidas.
- Mensaje de error.

Hoy `processed_count` es poco expresivo para el `SyncStats` actual.

## Estado general del backend

### Lo que está bien

- Estructura simple y entendible.
- Módulo `news` encapsulado.
- Uso de interfaces para extractor y classifier.
- PostgreSQL con constraints de unicidad.
- Endpoint manual de sync protegido.
- Listado público separado del admin.
- Worker separado del API.
- Config centralizada.
- CORS y router con chi.

### Lo que está medio flojo

- No hay ejecución automática real.
- No hay registro de corridas del worker.
- No hay tests.
- No hay migrator integrado documentado.
- No hay observabilidad seria.
- No hay autenticación robusta para admin.
- No hay validación fuerte de categorías/status desde DB.
- Hay strings con encoding roto.
- Variables de config de OpenAI están declaradas pero no implementadas.

### Lo que falta para producción

#### 1. Scheduler real para el worker

Opciones:

##### Opción A: Cron externo

Ejecutar:

```bash
go run ./cmd/worker
```

o el binario compilado cada cierto tiempo.

Pros:

- Simple.
- No cambia el código.
- Fácil en servidores Linux.

Contras:

- Menos portable.
- Logs y errores quedan dispersos si no se configura bien.

##### Opción B: Docker Compose con servicio worker programado

Agregar un contenedor que ejecute el worker con `cron`, `supercronic` o similar.

Pros:

- Más reproducible.
- Encaja con Docker.

Contras:

- Hay que armar imagen Docker.
- Hay que gestionar logs/health.

##### Opción C: Kubernetes CronJob

Ideal si el despliegue va a Kubernetes.

Pros:

- Nativo para jobs programados.
- Reintentos, historial y observabilidad.

Contras:

- Overkill si no hay Kubernetes.

##### Opción D: Scheduler dentro del API

Meter un ticker/gocron dentro de `cmd/api`.

Pros:

- Una sola app.
- Fácil para demo.

Contras:

- Peligroso si hay múltiples réplicas: todas podrían ejecutar el sync.
- Mezcla responsabilidades.
- Necesita lock distribuido para producción.

Recomendación: **Cron externo o CronJob**, no scheduler dentro del API, salvo demo/local.

#### 2. Persistir `job_runs`

Hoy el worker devuelve `SyncStats`, pero después esa información se pierde.

Recomendación:

- Crear métodos:
  - `StartJobRun`
  - `FinishJobRun`
  - `FailJobRun`
- Extender `job_runs` con columnas para el detalle de `SyncStats`.

Ejemplo de columnas:

```sql
extracted_count INTEGER DEFAULT 0,
new_items_count INTEGER DEFAULT 0,
duplicates_count INTEGER DEFAULT 0,
classified_count INTEGER DEFAULT 0,
inserted_count INTEGER DEFAULT 0,
failed_items_count INTEGER DEFAULT 0
```

#### 3. Mejorar deduplicación y métricas

Actualmente:

- Se consulta si existe.
- Si no existe, se inserta con `ON CONFLICT DO NOTHING`.
- Pero no se verifica si el insert realmente afectó filas.

Recomendación:

- Usar `CommandTag.RowsAffected()` en `InsertFromReel`.
- Devolver `inserted bool`.
- Si `RowsAffected() == 0`, contar como duplicate/conflict.

Eso evita métricas mentirosas en condiciones de carrera.

#### 4. Agregar revisión/aprobación real

Ya existe `needs_review`, pero no hay endpoints para:

- Publicar una noticia.
- Ocultarla.
- Editarla.
- Cambiar categoría/importancia.

Faltan endpoints admin como:

```txt
PATCH /api/admin/news/{id}
POST  /api/admin/news/{id}/publish
POST  /api/admin/news/{id}/hide
```

#### 5. Corregir encoding

Hay texto roto tipo:

```txt
Académico
inválido
Tecnología
```

Esto afecta:

- Mensajes de error.
- Categorías.
- Keywords.
- Clasificación.

Es prioritario porque puede romper clasificación y experiencia de usuario.

#### 6. Tests

Mínimo:

- Tests de `sanitizeInstagramUsername`.
- Tests de `extractReelShortcodes`.
- Tests de `normalizeText`.
- Tests de `SimpleNewsClassifier`.
- Tests de `Syncer` con extractor/repo fake.
- Tests HTTP para handlers principales.

Sin tests, tocar scraping/clasificación es caminar por una obra sin casco. Puede salir bien, sí, pero no es profesional.

#### 7. Observabilidad

Agregar:

- Logs estructurados.
- Correlation/request ID en operaciones admin.
- Métricas de sync.
- Última corrida visible desde endpoint admin.

Ejemplo:

```txt
GET /api/admin/news/sync/runs
GET /api/admin/news/sync/runs/latest
```

#### 8. Reemplazar scraping HTML si el proyecto lo necesita serio

El extractor HTML es frágil. Para producción, conviene evaluar:

- API oficial si aplica.
- Extractor propio autenticado si el alcance legal/técnico lo permite.

Como el proyecto no debe depender de proveedores externos, esta evolución debería priorizar API oficial o extractor propio autorizado.

## Flujo recomendado objetivo

```txt
Scheduler externo
  ↓
cmd/worker
  ↓
Start job_runs
  ↓
Extractor obtiene reels
  ↓
Por cada reel:
  - Normaliza source_media_id
  - Intenta insertar o detectar conflicto
  - Clasifica
  - Publica automáticamente si sirve como noticia
  - Guarda resultado
  ↓
Finish job_runs con SyncStats
  ↓
Publica/edita/oculta
```

## Prioridades sugeridas

### Prioridad alta

1. Definir cómo se agenda el worker.
2. Registrar corridas en `job_runs`.
3. Corregir encoding.
4. Ajustar `InsertFromReel` para devolver si insertó realmente.
5. Agregar endpoints para publicar/ocultar/editar noticias.

### Prioridad media

1. Tests de classifier, extractor y syncer.
2. Mejorar validaciones de input.
3. Agregar endpoint de últimas corridas.
4. Agregar logs estructurados.

### Prioridad baja

1. Cambiar extractor HTML por API oficial o extractor propio autorizado.
2. Agregar roles/permisos admin.
3. Dashboard de jobs.

## Conclusión

El backend está bien encaminado para una primera versión, pero todavía está en estado **MVP técnico**, no en estado productivo robusto.

Lo más importante: **agregar noticias funciona si alguien dispara el flujo**, pero la automatización no está resuelta. El worker existe, sí, pero no está programado. La validación de nuevas noticias existe por `source_media_id` más índice único, pero falta persistir auditoría, mejorar métricas y exponer herramientas admin para revisar qué pasó.

La arquitectura permite crecer, y eso es buenísimo. Pero hay que ponerse las pilas con la parte operativa: worker automático, job runs, tests y endpoints de revisión. Porque un sistema de noticias sin pipeline observable es como una obra sin planos de inspección: capaz se levanta, pero cuando algo falla nadie sabe dónde mirar.

