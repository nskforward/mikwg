# Разработка и тесты

Документ для тех, кто собирает mikwg из исходников или участвует в разработке.
Пользователям достаточно готового `rscgen` из
[релизов](https://github.com/nskforward/mikwg/releases/latest).

## Сборка и проверка

```bash
make test     # юнит- и интеграционные тесты (UDP round-trip)
make race     # с гонками
make bench    # бенчмарки трансформаций
make lint     # gofmt + go vet
make build    # кросс-сборка linux/arm64
make push     # сборка и публикация :$(VERSION) в docker.io/nskforward/mikwg
make push-release   # то же + тег latest
```

## Публикация образа

Публикация автоматизирована: пуш тега `vX.Y.Z` запускает GitHub Actions
(`.github/workflows/release.yml`), который прогоняет тесты, пушит
`nskforward/mikwg:X.Y.Z` и `:latest` и прикладывает `awg-converter-arm64.tar` к
GitHub Release. Нужны секреты репозитория `DOCKER_USERNAME`/`DOCKER_PASSWORD`
(пароль — Docker Hub access token с правом push).
Локально `make push` использует `DOCKER_USERNAME`/`DOCKER_PASSWORD` или
`docker login`.

Сборка образа без Docker-демона описана в [`advanced.md`](advanced.md).

## Структура пакетов

- `internal/awg` — трансформации проводного формата (ядро решения);
- `internal/config` — парсер `awg0.conf` и environment-переменных;
- `internal/proxy` — UDP-прокси;
- `cmd/awg-converter`, `cmd/rscgen`, `cmd/imagetool`.

Спецификация формата с ссылками на исходники amneziawg-go —
[`protocol-notes.md`](protocol-notes.md). Безопасные операции на RouterOS —
[`safe-operations.md`](safe-operations.md).

## Производительность

Все data-пакеты проходят через userspace-прокси (переупаковка без криптографии).
Hot path конвертера оптимизирован под минимум работы с памятью:

- входящий transport: точный пре-чек длины handshake (обычные data-пакеты не
  пробуются как init/resp/cookie) и расшифровка заголовка **in-place** в приёмном
  буфере — без `make+copy`;
- исходящий transport: кадрирование **in-place** с запасом `S4` байт слева
  (`WrapTransportInPlace`) — тело пакета не копируется;
- паддинг S1–S4 берётся из буферизованного `crypto/rand` (без syscall на пакет);
- адрес роутера хранится как `netip.AddrPort` — без аллокаций на пакет.

Микробенчмарки трансформаций (Apple M5 Pro, Go 1.26) — до → после:

| Путь | До | После |
|---|---|---|
| inbound transport | 795 ns/op, 2944 B/op, 5 allocs/op | 178 ns/op, 384 B/op, 1 alloc/op |
| outbound transport (hot) | 497 ns/op, 1944 B/op, 3 allocs/op | 167 ns/op, 384 B/op, 1 alloc/op |
| handshake initiation (cold) | 251 ns/op, 544 B/op, 2 allocs/op | 202 ns/op, 384 B/op, 1 alloc/op |

Остаётся ровно одна аллокация на пакет — `chacha20.Cipher` (384 B). Её планируется
снять собственной block-функцией ChaCha20 с предвычисленным ключом (будущая
оптимизация hot path).
