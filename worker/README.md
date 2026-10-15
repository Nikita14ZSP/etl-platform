# Worker

Go-воркер ETL-платформы. Забирает задания из Redis Streams, выполняет их и подтверждает.

Сейчас готово: чтение из очереди, разбор и проверка сообщения, основной цикл.
Операторов (bash, python, sql, http) пока нет — задание только пишется в лог.

## Структура

```
worker/
├── cmd/
│   ├── worker/       точка входа воркера
│   └── enqueue/      утилита для отправки тестовых заданий
├── internal/
│   ├── config/       настройки из переменных окружения
│   ├── logging/      настройка slog
│   ├── queue/        Redis Streams: consumer group, чтение, ack
│   └── runner/       основной цикл обработки
└── task/             сообщение очереди (контракт с планировщиком)
```

`task` лежит не в `internal`, чтобы его могли импортировать операторы из `operators/`.

## Запуск локально

Нужны Go 1.23+ и Redis.

```bash
docker run -d --name etl-redis -p 6379:6379 redis:7-alpine

cd worker
REDIS_URL=redis://localhost:6379/0 LOG_FORMAT=text make run
```

В другом терминале отправить задание:

```bash
cd worker
make enqueue CMD='echo hello'
```

В логах воркера появятся `task started` и `task finished`.

## Docker

Образ собирается из корня репозитория, потому что `go.mod` лежит там:

```bash
docker build -f worker/Dockerfile -t etl-worker:dev .
```

В образе есть и `worker`, и `enqueue`.

## Переменные окружения

| Переменная | По умолчанию | Описание |
|---|---|---|
| `REDIS_URL` | — | адрес Redis, обязательная |
| `WORKER_ID` | `<hostname>-<pid>` | имя потребителя в consumer group, должно быть уникальным |
| `TASK_STREAM` | `etl:tasks` | стрим с заданиями |
| `CONSUMER_GROUP` | `etl-workers` | consumer group воркеров |
| `READ_BLOCK` | `5s` | сколько ждать новое задание в одном запросе к Redis |
| `CONCURRENCY` | `1` | число одновременных задач (пул пока не реализован) |
| `SHUTDOWN_TIMEOUT` | `30s` | сколько ждать текущие задачи при остановке (пока не используется) |
| `PG_DSN` | — | Postgres для статусов задач (пока не используется) |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | `json` | `json` или `text` |

## Формат сообщения

Планировщик кладёт в стрим запись с полем `payload`, в котором JSON по контракту из концепции:

```bash
redis-cli XADD etl:tasks '*' payload "$(cat worker/task/testdata/task_message_v1.json)"
```

Пример сообщения — [task/testdata/task_message_v1.json](task/testdata/task_message_v1.json).
Сейчас поддерживается `schema_version: 1`, неизвестные поля игнорируются.

Что воркер делает с сообщением:

- битый JSON или не хватает обязательных полей — пишет ошибку в лог и подтверждает;
- задача выполнилась или упала — подтверждает (повторы делает планировщик);
- воркер остановили посреди задачи — не подтверждает, сообщение остаётся в pending.

## Тесты и линтер

```bash
cd worker
make test
make lint
```

Тесты очереди ходят в настоящий Redis (`TEST_REDIS_URL`, по умолчанию `redis://localhost:6379/15`)
и пропускаются, если он недоступен.
