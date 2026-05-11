# Ejecución automática del worker de noticias

El proyecto tiene dos formas de sincronizar noticias:

1. Scheduler interno del API, controlado por `NEWS_SYNC_INTERVAL_MINUTES`.
2. Worker one-shot, útil para cron externo o jobs de plataforma.

El worker de noticias es un proceso one-shot:

```bash
go run ./cmd/worker
```

Cada ejecución sincroniza Instagram una vez, registra la corrida en `job_runs` y termina.

## Scheduler interno del API

```env
NEWS_SYNC_INTERVAL_MINUTES=60
NEWS_SYNC_ON_STARTUP=false
NEWS_RECENT_DAYS=7
```

- `NEWS_SYNC_INTERVAL_MINUTES`: cada cuántos minutos corre la sincronización.
- `NEWS_SYNC_INTERVAL_MINUTES=0`: deshabilita el scheduler interno.
- `NEWS_SYNC_ON_STARTUP=true`: corre una sincronización al iniciar el servidor.
- `NEWS_RECENT_DAYS`: ventana de recencia aceptada.

Usalo en despliegues de una sola réplica. Si hay múltiples réplicas, preferí cron externo/CronJob o agregá lock distribuido.

## Stories de Instagram

El backend no soporta Stories actualmente.

Motivo: Instagram Stories no son confiables vía HTML público sin autenticación, y este proyecto no debe depender de proveedores externos para resolverlas. La sincronización automática queda limitada a Reels públicos.

Si una Story debe aparecer como noticia, cargala manualmente desde el endpoint admin `POST /api/admin/news`.

## Opción recomendada: cron del servidor

Ejemplo cada 30 minutos:

```cron
*/30 * * * * cd /ruta/cbtic-backend && /ruta/cbtic-worker >> /var/log/cbtic-news-worker.log 2>&1
```

Tradeoff:

- Simple y suficiente para VPS/servidor único.
- Requiere que el deploy compile o instale el binario `cbtic-worker`.

## Opción recomendada en Kubernetes: CronJob

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

Tradeoff:

- Mejor observabilidad y reintentos.
- Solo tiene sentido si el proyecto ya corre en Kubernetes.

## Opción para desarrollo: endpoint manual protegido

```bash
curl -X POST http://localhost:8080/api/admin/news/sync \
  -H "Authorization: Bearer dev-secret"
```

Tradeoff:

- Excelente para probar.
- No debería ser la automatización principal porque depende de una request HTTP larga.

## Cómo validar que corrió

Última corrida:

```bash
curl http://localhost:8080/api/admin/news/sync/runs/latest \
  -H "Authorization: Bearer dev-secret"
```

Historial:

```bash
curl "http://localhost:8080/api/admin/news/sync/runs?limit=20" \
  -H "Authorization: Bearer dev-secret"
```

## Cómo validar noticias nuevas

El sistema valida duplicados en dos niveles:

1. Aplicación: busca si ya existe `(source = instagram, source_media_id = shortcode)`.
2. Base de datos: índice único `(source, source_media_id)` más `ON CONFLICT DO NOTHING`.

Además, el insert ahora reporta si realmente insertó una fila. Si hubo una carrera y PostgreSQL ignoró el insert por conflicto, el sync lo cuenta como duplicado y no como insertado.
