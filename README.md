# mikwg — AmneziaWG 3.1 на MikroTik RouterOS

Клиент **AmneziaWG 3.1** для роутеров MikroTik (RouterOS 7.24+, ARM64), который
использует **штатный kernel WireGuard** роутера и компактный контейнер-конвертер
(`awg-converter`). На выходе — полноценный AmneziaWG 3.1 без выноса секретов за
пределы RouterOS и без потери производительности ядра на шифровании.

## Как это работает

```
[LAN] → маршруты → WireGuard (ядро RouterOS; приватный ключ только здесь)
                      │ peer endpoint = 10.99.0.2:51820 (veth)
                      ▼
            container: awg-converter (Go, статический бинарник ~2.5 МБ)
              · S1–S4 (префикс-паддинг), H1–H4 (идентификаторы типа)
              · HeaderProtectionKey (ChaCha20, nonce из S-паддинга)
              · junk-пакеты Jc и CPS I1–I5
              · обратное срезание слоёв
                      │ veth → bridge → masquerade → WAN
                      ▼
            [AmneziaWG 3.1 сервер]
```

AmneziaWG не меняет криптографию WireGuard: все его механизмы — внешние слои над
пакетом. Конвертер «переодевает» пакеты на лету, а AEAD/MAC остаются
нетронутыми (MAC считаются над каноническим сообщением — см.
[`docs/protocol-notes.md`](docs/protocol-notes.md)).

**Модель безопасности:** конвертеру доступны только параметры обфускации (включая
`HeaderProtectionKey`) и метаданные трафика. Приватный ключ WireGuard существует
только в `/interface/wireguard` RouterOS и в контейнер не передаётся.

## Требования

- MikroTik ARM64 (проверялось на RB5009 и hAP ax³), RouterOS 7.24+
- **Рабочая конфигурация роутера** — mikwg не выполняет factory reset и
  не заменяет вашу конфигурацию, а дополняет её
- Пакет `container` той же версии RouterOS
- ≥ 64 МБ свободного места во flash; носитель с `awg0.conf`
  (по умолчанию `/usb1/awg-config/awg0.conf`)
- Конфиг AmneziaWG 3.1 (поля `S1-S4`, `H1-H4`, `HeaderProtectionKey`, `Jc/Jmin/Jmax`, `I1-I5`)
- Для выборочной маршрутизации — address-list `to_vpn_list` (создаётся заранее
  или переиспользуется, если уже существует)
- Go 1.26+ для сборки (Docker — опционально)

## Быстрый старт

### 1. Сборка образа

С Docker:
```bash
./build.sh                      # → awg-converter-arm64.tar
```

Без запущенного Docker-демона (использует in-process сборщик образа):
```bash
./build-nodocker.sh             # → awg-converter-arm64.tar
```

### 2. Генерация RouterOS-скрипта

`rscgen` читает **реальный** `awg0.conf` (вместе с секретами) и пишет скрипт
развёртывания:
```bash
go run ./cmd/rscgen -conf awg0.conf -out routeros.generated.rsc
```
Полезные флаги: `-addr-list` (умолч. `to_vpn_list`), `-rt-table` (умолч.
`to_vpn_table`), `-conn-mark` (умолч. `to_vpn_mark`), `-wan-iface-list` (умолч.
`WAN`), `-wan-gw` (next-hop для антилуп-маршрута; по умолчанию определяется
автоматически из основного default-маршрута — **указывайте IP шлюза, а не
интерфейс**), `-tar`, `-wg-name`, `-wg-port`.

> ⚠️ Антилуп-маршрут до IP сервера должен указывать на **реальный next-hop**
> (`gateway=192.168.1.1`), а не на интерфейс (`gateway=ether1`). Interface-route
> заставляет RouterOS ARP-ить публичный IP прямо в LAN-сегменте WAN, что на
> обычном ethernet-аплинке не срабатывает и полностью ломает туннель.

Файл `*.generated.rsc` содержит приватный ключ — он в `.gitignore`, не коммитьте его.

### 3. Подготовка роутера (выполняется пользователем самостоятельно)

> ⚠️ mikwg **никогда не сбрасывает роутер**: ни `rscgen`, ни сгенерированный
> скрипт не содержат `reset-configuration`. Установка идёт поверх вашей рабочей
> конфигурации. Перед началом прочитайте
> [`docs/safe-operations.md`](docs/safe-operations.md).

Выполните один раз вручную:

1. Сделайте и **скачайте на рабочую станцию** бэкап:
   ```rsc
   /system/backup/save name=pre-mikwg
   /export file=pre-mikwg-export
   ```
2. Загрузите в Files пакет `container-7.24-arm64.npk` и перезагрузите роутер.
3. Включите режим контейнеров и перезагрузитесь:
   ```rsc
   /system/device-mode/update container=yes
   ```
4. Загрузите в Files образ `awg-converter-arm64.tar` и конфиг `awg0.conf`
   (по умолчанию `/usb1/awg-config/awg0.conf`).
5. Убедитесь, что доступ к управлению переживёт изменения (LAN-порт или
   Winbox-by-MAC).

### 4. Установка

```text
/import file=routeros.generated.rsc
```

Скрипт **идемпотентен** (повторный импорт безопасен) и:

- создаёт veth/bridge/NAT для контейнера, монтирует конфиг read-only;
- добавляет и запускает контейнер `awg-converter`;
- поднимает WireGuard с endpoint на контейнер (приватный ключ остаётся в RouterOS);
- добавляет антилуп-маршрут до IP сервера через реальный WAN next-hop;
- добавляет srcnat в туннель (`out-interface=awg`), чтобы трафик роутера и
  LAN-клиентов уходил к серверу с внутренним адресом клиента (см. следующий раздел);
- **переиспользует** существующие `to_vpn_list` и `to_vpn_table` для выборочной
  маршрутизации (см. следующий раздел);
- ставит watchdog и `start-on-boot`.

**Дефолтный маршрут роутера не меняется.**

## Выборочная маршрутизация

Через туннель идёт только трафик к адресам из списка `to_vpn_list` — это
единственная точка управления маршрутизацией, default route не трогается:

```rsc
# направить подсеть/адрес в VPN
/ip/firewall/address-list/add list=to_vpn_list address=1.2.3.0/24
# убрать
/ip/firewall/address-list/remove [find list=to_vpn_list address="1.2.3.0/24"]
# направить ВЕСЬ трафик через туннель
/ip/firewall/address-list/add list=to_vpn_list address=0.0.0.0/0
```

Как это работает:

- `mangle` в `prerouting` маркирует новые соединения к `to_vpn_list`
  (`mark-connection to_vpn_mark`), затем всем их пакетам ставит routing-mark
  `to_vpn_table`; в этой таблице `0.0.0.0/0` указывает на `awg`;
- `mangle` в `output` делает то же для трафика самого роутера (например, DNS-запросов);
- трафик к IP AmneziaWG-сервера явно исключён из маркировки (anti-loop);
- трафик, приходящий **из** туннеля (`in-interface=awg`), исключён из маркировки
  маршрута и уходит в LAN через main-таблицу. **Это обязательно:** иначе ответы
  сервера (у которых тот же `to_vpn_mark`) повторно маркируются и уходят обратно
  в туннель — получается reply-loop: TCP-handshake завершается, но данные до
  клиента не доходят;
- для TCP в туннель клампится MSS (`new-mss=1360`), чтобы LAN-клиенты не слали
  сегменты больше MTU туннеля;
- трафик, уходящий в туннель, маскарадится в `out-interface=awg` (srcnat), чтобы
  к серверу он приходил с адресом клиента (`10.8.2.5`), а не с WAN/LAN-адресом.
  **Это обязательно:** серверная конфигурация Amnezia из «одноустройственного»
  профиля принимает на пире только адрес клиента, поэтому пакеты с «чужим» src
  молча отбрасываются (cryptokey routing) — handshake при этом проходит, а data
  нет;
- правило(а) FastTrack получают `connection-mark=!to_vpn_mark`, иначе FastTrack
  обходит `mangle` и ломает policy routing (скрипт делает это идемпотентно и не
  трогает правило, где такое исключение уже есть).

Список `to_vpn_list`, таблица `to_vpn_table` и connection-mark `to_vpn_mark`
создаются заранее или переиспользуются, если уже существуют; имена
переопределяются флагами `rscgen` (`-addr-list`, `-rt-table`, `-conn-mark`).

## Конфигурация конвертера

Источник параметров — смонтированный `awg0.conf` (`PrivateKey` пропускается и не
используется). Флаги:

| Флаг | По умолчанию | Описание |
|---|---|---|
| `-conf` | `/etc/awg/awg0.conf` | путь к конфигу |
| `-listen` | `0.0.0.0:51820` | адрес, куда шлёт kernel WireGuard (endpoint пира) |
| `-upstream` | из `Endpoint` conf | адрес AmneziaWG-сервера |
| `-jitter` | `false` | тайминговый джиттер initiation-пакетов |
| `-v` | `false` | подробный лог |
| `-version` | | версия |

## Что воспроизводится из AmneziaWG 3.1

| Механизм | Статус |
|---|---|
| S1–S4 (паддинг перед сообщением) | ✅ |
| H1–H4 (идентификаторы типов) | ✅ |
| HeaderProtectionKey (ChaCha20, nonce из S-паддинга) | ✅ |
| Jc/Jmin/Jmax (мусорные пакеты) | ✅ |
| I1–I5 / CPS (пакеты-приманки, теги `<b>/<r>/<rc>/<rd>/<t>/<dz>`) | ✅ |
| Тайминговая обфускация | ⚠️ частично (дефолты kernel-WG уже в диапазонах; опциональный джиттер) |
| ContentPaddingAddition (паддинг внутри шифротекста) | ❌ исходяще, не требуется (односторонний) |
| RandomTrailers (3.1) | ❌ на сервере должен быть выключен |

## Разработка и тесты

```bash
make test     # юнит- и интеграционные тесты (UDP round-trip)
make race     # с гонками
make bench    # бенчмарки трансформаций
make lint     # gofmt + go vet
make build    # кросс-сборка linux/arm64
```

Пакеты:
- `internal/awg` — трансформации проводного формата (ядро решения);
- `internal/config` — парсер `awg0.conf`;
- `internal/proxy` — UDP-прокси;
- `cmd/awg-converter`, `cmd/rscgen`, `cmd/imagetool`.

Спецификация формата с ссылками на исходники amneziawg-go —
[`docs/protocol-notes.md`](docs/protocol-notes.md). Безопасные операции на
RouterOS — [`docs/safe-operations.md`](docs/safe-operations.md).

## Производительность

TBD (заполняется по итогам нагрузочного теста на роутере). Все data-пакеты
проходят через userspace-прокси (лёгкая переупаковка без криптографии); целимся в
200–400 Мбит/с на RB5009 / hAP ax³.

## Ограничения

- Только RouterOS с поддержкой контейнеров (ARM/ARM64/x86; не MIPS).
- Один peer (один AWG-сервер).
- Маршрутизация выборочная: через туннель идёт только `to_vpn_list`, остальное —
  напрямую. Чтобы завернуть всё, добавьте `0.0.0.0/0` в список.
- Туннель IPv4-only: IPv6-трафик через него не пойдёт. Если нужен полный
  IPv6-нет-лок, отключите IPv6 на LAN.
- `ContentPaddingAddition` и точная тайминговая мимикрия не воспроизводятся.

## Лицензия

GPLv3 (см. [`LICENSE`](LICENSE)). Логика трансформаций сверена и заимствована из
[`amnezia-vpn/amneziawg-go`](https://github.com/amnezia-vpn/amneziawg-go) (GPLv2),
что совместимо с GPLv3.

План работ — [`PLAN.md`](PLAN.md).
