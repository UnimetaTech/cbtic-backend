# CBTIC Backend

Backend en Go para servir noticias institucionales, crear noticias manuales y sincronizar publicaciones desde Instagram Reels.

## Estado actual, sin humo

El servidor HTTP funciona como API principal y el worker existe como proceso separado.

**Importante:** el servidor HTTP puede ejecutar la sincronización automáticamente si `NEWS_SYNC_INTERVAL_MINUTES` es mayor a `0`. Por defecto en `.env.example` queda cada `60` minutos.

También se puede ejecutar manualmente de estas formas:

1. Manualmente con `go run ./cmd/worker`.
2. Manualmente desde HTTP con `POST /api/admin/news/sync`.
3. Automáticamente desde el API con el scheduler interno.
4. Automáticamente con un scheduler externo, por ejemplo cron, Kubernetes CronJob, GitHub Actions, Railway cron, Render cron job, etc.

Si usás más de una réplica del API en producción, preferí scheduler externo o agregá lock distribuido. Si no, cada réplica podría intentar sincronizar.

## Requisitos

- Go `1.26.2` según `go.mod`.
- Docker y Docker Compose para levantar PostgreSQL local.
- PostgreSQL.
- Opcional: `goose` para ejecutar migraciones.

## Variables de entorno

Copiá el ejemplo:

```bash
cp .env.example .env
```

En Windows PowerShell:

```powershell
Copy-Item .env.example .env
```

Variables principales:

```env
APP_ENV=development
PORT=8080

DATABASE_URL=postgresql://cbtic:cbtic@127.0.0.1:5433/cbtic_db?sslmode=disable
CORS_ORIGIN=http://localhost:5173

INSTAGRAM_TARGET_USERNAME=nombre_de_la_cuenta

NEWS_SYNC_SECRET=dev-secret
NEWS_RECENT_DAYS=7
NEWS_SYNC_INTERVAL_MINUTES=60
NEWS_SYNC_ON_STARTUP=false
NEWS_FEATURED_LIMIT=3

OLLAMA_ENABLED=false
OLLAMA_BASE_URL=http://192.168.100.103:11434
OLLAMA_MODEL=qwen2.5:7b-instruct-q4_K_M
```

Variables reservadas para futuras integraciones:

```env
OPENAI_API_KEY=
OPENAI_MODEL=gpt-4.1-mini
```

Actualmente el backend usa scraping HTML simple para Reels. La clasificación puede usar un modelo local de Ollama si `OLLAMA_ENABLED=true`; si Ollama no está disponible, cae al clasificador local por palabras clave.

## Levantar base de datos local

```bash
docker compose up -d
```

Esto levanta PostgreSQL en:

```txt
127.0.0.1:5433
```

Credenciales locales:

```txt
user: cbtic
password: cbtic
database: cbtic_db
```

## Ejecutar migraciones

El proyecto tiene migraciones en:

```txt
migrations/
```

Si usás `goose`, el comando sería:

```bash
goose -dir migrations postgres "postgresql://cbtic:cbtic@127.0.0.1:5433/cbtic_db?sslmode=disable" up
```

En PowerShell:

```powershell
goose -dir migrations postgres "postgresql://cbtic:cbtic@127.0.0.1:5433/cbtic_db?sslmode=disable" up
```

Si no tenés `goose`, instalalo:

```bash
go install github.com/pressly/goose/v3/cmd/goose@latest
```

> Nota: el repositorio no trae un comando propio de migración todavía. Usar `goose` es la forma esperada por el formato de los archivos SQL (`-- +goose Up`, `-- +goose Down`).

## Ejecutar servidor HTTP

```bash
go run ./cmd/api
```

Por defecto escucha en:

```txt
http://localhost:8080
```

Healthcheck:

```bash
curl http://localhost:8080/health
```

Respuesta esperada:

```json
{"status":"ok"}
```

## Ejecutar worker manualmente

```bash
go run ./cmd/worker
```

El worker:

1. Lee `.env`.
2. Conecta a PostgreSQL.
3. Busca los últimos Reels de `INSTAGRAM_TARGET_USERNAME`.
4. Deduplica por `source + source_media_id`.
5. Clasifica cada Reel.
6. Si Ollama está habilitado, valida si sirve como noticia y redacta `title`, `summary` y `body`.
7. Publica automáticamente todo Reel clasificado como noticia.
8. Mantiene la `confidence` como dato de auditoría, pero no la usa para bloquear la publicación.
9. Registra la corrida en `job_runs`.
10. Termina.

Es un proceso **one-shot**: corre una vez y finaliza.

## Ejecutar todo en desarrollo

Terminal 1:

```bash
docker compose up -d
```

Terminal 2:

```bash
goose -dir migrations postgres "postgresql://cbtic:cbtic@127.0.0.1:5433/cbtic_db?sslmode=disable" up
```

Terminal 3:

```bash
go run ./cmd/api
```

Opcional, para sincronizar noticias manualmente:

```bash
go run ./cmd/worker
```

O usando el endpoint admin:

```bash
curl -X POST http://localhost:8080/api/admin/news/sync \
  -H "Authorization: Bearer dev-secret"
```

## Automatizar la sincronización

El API trae scheduler interno configurable:

```env
NEWS_SYNC_INTERVAL_MINUTES=60
NEWS_SYNC_ON_STARTUP=false
```

- `NEWS_SYNC_INTERVAL_MINUTES=60`: sincroniza cada 60 minutos.
- `NEWS_SYNC_INTERVAL_MINUTES=0`: deshabilita el scheduler interno.
- `NEWS_SYNC_ON_STARTUP=true`: ejecuta una sincronización al iniciar el API.

Para producción con múltiples réplicas, lo más seguro sigue siendo scheduler externo.

### Opción simple: cron

Ejemplo cada 30 minutos:

```cron
*/30 * * * * cd /ruta/cbtic-backend && /ruta/cbtic-worker >> /var/log/cbtic-news-worker.log 2>&1
```

### Opción con Kubernetes: CronJob

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: cbtic-news-worker
spec:
  schedule: "*/30 * * * *"
  jobTemplate:
    spec:
      template:
        spec:
          restartPolicy: OnFailure
          containers:
            - name: worker
              image: cbtic-backend:latest
              command: ["/app/worker"]
              envFrom:
                - secretRef:
                    name: cbtic-backend-env
```

Más detalles en:

```txt
docs/news-worker-scheduling.md
```

## Autenticación admin

Los endpoints admin requieren:

```txt
Authorization: Bearer <NEWS_SYNC_SECRET>
```

Ejemplo local:

```txt
Authorization: Bearer dev-secret
```

## Endpoints públicos

### `GET /health`

Verifica si el servidor está vivo.

```bash
curl http://localhost:8080/health
```

### `GET /api/news`

Lista noticias publicadas.

Query params:

- `category`
- `importance`
- `limit`
- `offset`

Ejemplo:

```bash
curl "http://localhost:8080/api/news?limit=12&offset=0"
```

Ejemplo con filtros:

```bash
curl "http://localhost:8080/api/news?category=Institucional&importance=media"
```

Respuesta:

```json
{
  "data": [
    {
      "id": 1,
      "title": "Título",
      "summary": "Resumen",
      "body": "Contenido",
      "imageUrl": "https://example.com/image.jpg",
      "thumbnailUrl": "https://example.com/image.jpg",
      "publishedAt": "2026-04-29T12:00:00Z",
      "reelUrl": "https://www.instagram.com/reel/...",
      "category": "Institucional",
      "importance": "media",
      "source": "instagram"
    }
  ],
  "pagination": {
    "limit": 12,
    "offset": 0,
    "total": 1
  }
}
```

### `GET /api/news/{id}`

Obtiene una noticia publicada.

```bash
curl http://localhost:8080/api/news/<id>
```

## Endpoints admin

Todos requieren header:

```bash
-H "Authorization: Bearer dev-secret"
```

### `GET /api/admin/news`

Lista todas las noticias: publicadas, borradores, ocultas y pendientes de revisión.

```bash
curl http://localhost:8080/api/admin/news \
  -H "Authorization: Bearer dev-secret"
```

### `POST /api/admin/news`

Crea una noticia manual.

```bash
curl -X POST http://localhost:8080/api/admin/news \
  -H "Authorization: Bearer dev-secret" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Nueva noticia institucional",
    "summary": "Resumen breve de la noticia",
    "body": "Contenido completo de la noticia",
    "imageUrl": "https://example.com/image.jpg",
    "reelUrl": "manual://noticia-local",
    "category": "Institucional",
    "importance": "media",
    "status": "published"
  }'
```

Campos:

- `title`: obligatorio.
- `summary`: obligatorio.
- `body`: opcional.
- `imageUrl`: opcional, debe ser HTTP(S).
- `reelUrl`: opcional, debe ser HTTP(S) o `manual://...`.
- `category`: opcional, default `Institucional`.
- `importance`: opcional, default `media`.
- `status`: opcional, default `published`.

Valores válidos para `status`:

```txt
published
draft
hidden
needs_review
```

Valores válidos para `importance`:

```txt
alta
media
baja
```

Valores válidos para `category`:

```txt
Académico
Eventos
Investigación
Deportes
Tecnología
Cultura
Bienestar
Institucional
Otro
```

### `PATCH /api/admin/news/{id}`

Edita campos de una noticia.

```bash
curl -X PATCH http://localhost:8080/api/admin/news/<id> \
  -H "Authorization: Bearer dev-secret" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Título actualizado",
    "summary": "Resumen actualizado",
    "category": "Eventos",
    "importance": "alta",
    "status": "needs_review"
  }'
```

### `POST /api/admin/news/{id}/publish`

Publica una noticia.

```bash
curl -X POST http://localhost:8080/api/admin/news/<id>/publish \
  -H "Authorization: Bearer dev-secret"
```

### `POST /api/admin/news/{id}/hide`

Oculta una noticia.

```bash
curl -X POST http://localhost:8080/api/admin/news/<id>/hide \
  -H "Authorization: Bearer dev-secret"
```

### `POST /api/admin/news/sync`

Ejecuta sincronización de Instagram desde HTTP.

```bash
curl -X POST http://localhost:8080/api/admin/news/sync \
  -H "Authorization: Bearer dev-secret"
```

Respuesta:

```json
{
  "extracted": 10,
  "newItems": 2,
  "duplicates": 8,
  "classified": 2,
  "inserted": 2,
  "failedItems": 0
}
```

### `POST /api/admin/news/ai/reprocess`

Reprocesa noticias ya escaneadas desde Instagram usando Ollama. La IA valida si cada item realmente sirve como noticia académica, de eventos o institucionalmente importante, y reescribe `title`, `summary` y `body`.

```bash
curl -X POST "http://localhost:8080/api/admin/news/ai/reprocess?limit=20" \
  -H "Authorization: Bearer dev-secret"
```

Respuesta:

```json
{
  "scanned": 20,
  "processed": 18,
  "published": 12,
  "rejected": 6,
  "failed": 2
}
```

Los items aceptados quedan `published`; los rechazados por la IA quedan en `needs_review` y no aparecen en `/api/news`.

### `GET /api/admin/news/sync/runs`

Lista ejecuciones del worker.

```bash
curl "http://localhost:8080/api/admin/news/sync/runs?limit=20" \
  -H "Authorization: Bearer dev-secret"
```

### `GET /api/admin/news/sync/runs/latest`

Obtiene la última ejecución del worker.

```bash
curl http://localhost:8080/api/admin/news/sync/runs/latest \
  -H "Authorization: Bearer dev-secret"
```

## Cómo saber si se agregaron noticias nuevas

El sync devuelve:

- `extracted`: Reels encontrados.
- `newItems`: Reels que no existían antes.
- `duplicates`: Reels ya existentes o insert ignorado por conflicto.
- `classified`: Reels clasificados.
- `inserted`: noticias realmente insertadas.
- `skippedOld`: elementos descartados por no pertenecer a la ventana reciente.
- `skippedNotNews`: elementos descartados porque no tienen suficientes señales de noticia.
- `failedItems`: elementos que fallaron.

Además, cada ejecución queda registrada en `job_runs` y se puede consultar con:

```bash
curl http://localhost:8080/api/admin/news/sync/runs/latest \
  -H "Authorization: Bearer dev-secret"
```

La deduplicación se hace por:

```txt
source + source_media_id
```

En Instagram, `source_media_id` es el shortcode del Reel.

## Ventana de noticias recientes

El backend solo acepta contenido dentro de la ventana:

```env
NEWS_RECENT_DAYS=7
```

Si el extractor no puede obtener fecha de publicación del Reel, el item se descarta. Esto es intencional: si no sabemos cuándo fue publicado, no podemos afirmar que es reciente.

## Validación de “sirve como noticia”

Antes de insertar, el clasificador evalúa:

- Caption no vacío.
- Coincidencias con palabras clave institucionales.
- Marcadores de intención noticiosa como convocatoria, comunicado, evento, jornada, taller, inscripciones abiertas, etc.

Si no alcanza señales mínimas, no se inserta y se cuenta como `skippedNotNews`.

## Historias de Instagram

El backend **no soporta Stories actualmente**.

Y esto no es capricho: Instagram Stories no son confiables vía HTML público sin autenticación. Como el proyecto no debe usar proveedores externos, el flujo automático queda limitado a Reels públicos.

Si una Story debe convertirse en noticia, por ahora cargala manualmente con:

```txt
POST /api/admin/news
```

## Tests

```bash
go test ./...
```

## Documentación adicional

- `docs/news-backend-analysis.md`: análisis técnico del backend de noticias.
- `docs/news-worker-scheduling.md`: opciones para automatizar el worker.

## Notas de arquitectura

- `cmd/api`: servidor HTTP.
- `cmd/worker`: sincronizador one-shot.
- `internal/modules/news`: módulo de noticias.
- `internal/platform/database`: conexión a PostgreSQL.
- `internal/platform/httpserver`: router HTTP.

La separación API/worker es correcta. El API puede programar sincronizaciones simples, y `cmd/worker` queda como alternativa one-shot para cron externo o jobs de plataforma.
