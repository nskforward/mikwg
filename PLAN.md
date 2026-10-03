# mikwg — план реализации AmneziaWG 3.1-клиента на RouterOS 7.24+ (kernel WireGuard + container-конвертер)

> Рабочий план проекта. Статус — в разделе «Статус реализации» ниже: этапы 0–5 и 7
> выполнены, этап 6 (приёмка) начат. **Этап 5 развёрнут на роутере, блокер data-path закрыт** (см.
> «✅ Блокер закрыт: сессия 2026-10-03 (продолжение)»). Причина была не в
> транспорте AmneziaWG, а в трёх ошибках развёртывания: антилуп-маршрут на
> интерфейс вместо next-hop (ARP публичного IP в LAN), отсутствие srcnat в
> туннель (сервер отбрасывал пакеты с «чужим» src) и reply-loop маркированного
> маршрута (ответы сервера уходили обратно в туннель). Все три исправлены на
> роутере и в `rscgen`. **Подход изменён:** mikwg больше не выполняет factory reset —
> роутер готовит пользователь (см. README), маршрутизация в туннель выборочная,
> через существующие `to_vpn_list` / `to_vpn_table`. **Установка — из Docker Hub:**
> образ `nskforward/mikwg` (текущий релиз `1.1.1`, параметры через `/container/envs`)
> собирается без Docker-демона и
> публикуется GitHub Actions; роутер забирает его через `/container/add
> remote-image=`, загрузка tar в Files не требуется.

---

## Статус реализации (актуально на 2026-10-03)

| Этап | Статус | Что сделано / где лежит |
|---|---|---|
| 0. Каркас репозитория | ✅ выполнено | `go.mod`, `Makefile`, `.gitignore`, структура каталогов |
| 1. Спецификация протокола | ✅ выполнено | [`docs/protocol-notes.md`](docs/protocol-notes.md). **Главный риск снят:** MAC1/MAC2 считаются над каноническим WG-сообщением до шифрования заголовка ⇒ пересчёт MAC не нужен |
| 2. Конвертер | ✅ выполнено | `internal/awg`, `internal/config`, `internal/proxy`, `cmd/awg-converter`. `go vet`/`gofmt` чисто, тесты (включая сквозной UDP round-trip) зелёные |
| 3. Локальный интероп-стенд | 🟡 частично | вместо docker-стенда — in-process UDP round-trip тест (`internal/proxy/proxy_test.go`); боевой интероп с реальным сервером проверяем на роутере (Этап 5) |
| 4. Образ + rscgen | ✅ выполнено | `Dockerfile`, `build.sh`, `build-nodocker.sh`, `cmd/imagetool` (tar + **push в Docker Hub**), `cmd/rscgen` (registry-режим `-image` и оффлайн `-tar`), `.github/workflows/release.yml`. Образ публикуется как `nskforward/mikwg`; tar — оффлайн-опция. **Важно:** tar собирается с **несжатым** rootfs-слоем (`imagetool` → `static.NewLayer(..., OCIUncompressedLayer)`), иначе RouterOS 7.24 не импортирует образ (`error getting layer file / failed to load next entry`); тот же образ для реестра пушится с **gzip-слоем** (remote-image распаковывает слой при загрузке) |
| 5. Настройка RouterOS | 🟢 развёрнуто, data-path работает | объекты на роутере подняты (veth/bridge/container/WG/mangle/routes/watchdog), `to_vpn_list`/`to_vpn_table` переиспользованы, дефолт не тронут. Блокер data-path закрыт. **2026-10-03: контейнер мигрирован со старого tar на `remote-image=nskforward/mikwg:1.0.1`** (Docker Hub pull + extract — работает), контейнер running, handshake свежий, `fetch` адреса из `to_vpn_list` — HTTP 200. Осталось: приёмка (Этап 6) |
| 6. Приёмка | 🟡 начато | частично: рукопожатие, data-path (TCP через туннель) и переустановка из реестра проверены на роутере. Осталось: пропускная способность/CPU, pcap-форма, 24 ч стабильности |
| 7. README | ✅ выполнено | README/docs актуализированы: установка из Docker Hub, обновление, проверка, устранение неполадок (в т.ч. gzip-слой для `remote-image`); цифры производительности — после Этапа 6 |

**Инвентарь (уже готово):** рабочий конвертер + тесты; собранный arm64-тар;
генератор RouterOS-скрипта `rscgen` (идемпотентный, переиспользует
`to_vpn_list`/`to_vpn_table`); документация по протоколу и безопасным операциям.
Шаблон сброса `deploy/safe-reset.rsc` **удалён**: mikwg не сбрасывает роутер.

**Полученные данные с роутера (2026-10-03):** hAP ax³, RouterOS 7.24.5 (arm64),
1 ГБ RAM, flash 128 МиБ (свободно ~78 МиБ, bad-blocks 8.5%), пакет `container`
установлен, `device-mode container=yes`. Реальный `awg0.conf` прочитан
(`usb1/awg-config/awg0.conf`); секреты в репозиторий не попадают — хранятся на
роутере и локально вне git. На flash остались бэкапы `pre-mikwg.backup` и
`pre-mikwg-export.rsc`.

**«Хвосты» на роутере (переиспользовать, не создавать заново):**
- address-list `to_vpn_list` — уже заполнен; база выборочной маршрутизации;
- routing-table `to_vpn_table` — уже существует; наш дефолт `0.0.0.0/0 → awg`
  добавляется именно в неё.

---

## ⏭ Точка возобновления (доступ восстановлен)

**Никаких сбросов.** Установка идёт поверх рабочей конфигурации; роутер готовит
пользователь по чек-листу в [`README.md`](README.md) (бэкап, пакет `container`,
`device-mode`, загрузка файлов). `rscgen` и сгенерированный скрипт
идемпотентны и не содержат `reset-configuration`.

Порядок:

1. **Инвентаризация «хвостов» и состояния** (не меняя ничего): `/system resource print`,
   `/system package print`, `/system device-mode print`, `/interface print`,
   `/ip route print`, `/routing table print`,
   `/ip firewall address-list print where list=to_vpn_list`,
   `/ip route print where routing-table=to_vpn_table`,
   `/ip firewall mangle print`, `/ip firewall filter print`.
   Цель: убедиться, что `container` включён, есть место, заполнен `to_vpn_list`,
   существует `to_vpn_table`, и понять, нет ли уже наших/чужих правил
   маркировки и правила FastTrack.
2. **Убедиться, что доступ гарантирован** по правилам
   [`docs/safe-operations.md`](docs/safe-operations.md) (LAN-порт или
   Winbox-by-MAC; при необходимости — Safe Mode Ctrl-X). `run-after-reset` **не
   требуется**, т.к. сброс не выполняется.
3. **Проверить наличие на роутере** `/usb1/awg-config/awg0.conf` (образ роутер
   скачает сам из Docker Hub; tar нужен только для оффлайн-установки).
4. **Сгенерировать скрипт:**
   `go run ./cmd/rscgen -conf <реальный awg0.conf> -image nskforward/mikwg:1.0.1 -out routeros.generated.rsc`
   (реальный конфиг уже получен; хранится локально вне git). По умолчанию
   используются `to_vpn_list` / `to_vpn_table`; имена переопределяются флагами
   `-addr-list` / `-rt-table`.
5. **Выполнить Этап 5 по шагам**, проверяя после каждого: сеть контейнера →
   контейнер → WireGuard handshake → тест маршрутизации через `to_vpn_table`
   (пингом адреса из `to_vpn_list`; `/ping` не принимает `routing-table=`) →
   затем пополнить `to_vpn_list` и проверить с LAN-клиента.
6. **Этап 6 (приёмка):** скорость/CPU, pcap-форма трафика, 24 ч стабильности.
7. **Этап 7:** занести фактические цифры в README.

> Если конфигурация роутера окажется в нежелательном состоянии — восстановить из
> `pre-mikwg.backup` (`/system backup load name=pre-mikwg`, вручную, пользователем).
> Сброс через `reset-configuration` для установки mikwg **не используется**.

---

## ⏸ Сессия 2026-10-03: развёрнуто на роутере, data-path не проходит (точка траблшутинга)

> Здесь зафиксировано фактическое состояние на конец сессии, чтобы продолжить
> траблшутинг в новой. Доступ к роутеру: SSH `admin@192.168.88.1` (пароль у
> пользователя, в репозиторий не попадает); управляющая станция в LAN
> `192.168.88.0/24`. Дефолтный маршрут роутера не менялся, доступ не терялся.

### ✅ Блокер закрыт: сессия 2026-10-03 (продолжение)

Диагностика со стороны роутера показала, что причина не в транспорте AmneziaWG,
а в двух ошибках развёртывания. Обе исправлены на роутере и в `rscgen`.

**Первопричина 1 — антилуп-маршрут через интерфейс.** Маршрут
`185.255.134.26/32 gateway=ether1` был создан как interface-route, поэтому
RouterOS ARP-ил публичный IP прямо в LAN-сегменте `192.168.1.0/24`
(`/ip arp print` → `185.255.134.26 … status=failed`). Следствия: ping/traceroute
до сервера падали на самом роутере, WireGuard handshake не проходил
(`last-handshake` > 45 мин), а watchdog каждую минуту зря перезапускал контейнер.
LAN-клиенты при этом получали `Network is unreachable` (ICMP от роутера).

Исправление: `/ip/route/set [find comment="mikwg server via WAN"] gateway=192.168.1.1`
(реальный next-hop из основного default-маршрута). В `rscgen` теперь
используется автодетект шлюза из активного default-маршрута, а `-wan-gw`
принимает next-hop IP; interface-route больше не генерируется.

**Первопричина 2 — нет srcnat в туннель.** Трафик роутера/LAN уходил в туннель
с реальным src (`192.168.1.10` / `192.168.88.x`), а серверный peer
«одноустройственного» профиля принимает только `10.8.2.5/32` и молча дропает
остальное (cryptokey routing). Это в точности давало «handshake ходит, data
нет»: handshake-сообщения не содержат внутреннего IP, поэтому проходили.

Исправление: добавлен
`/ip/firewall/nat add chain=srcnat action=masquerade out-interface=awg comment="mikwg tunnel snat"`.
Это же правило теперь генерирует `rscgen`.

**Первопричина 3 — reply-loop маркированного маршрута.** Даже после первых двух
правок LAN-клиенты не получали данные: conntrack показывал `orig-packets=1`,
`repl-packets=7`, `tcp-state=syn-recv` — SYN уходил, ответы приходили, но клиент
их не видел. Причина: ответы сервера имеют тот же `connection-mark=to_vpn_mark`,
попадали под правило `mikwg: route to vpn` и **маршрутизировались обратно в
туннель** вместо LAN (main-таблица). Router-originated трафик (fetch роутера)
при этом работал, т.к. доставлялся локально, минуя forward.

Исправление: правило `chain=prerouting in-interface=awg action=accept passthrough=no
comment="mikwg: vpn return"`. Позже перенесено **в начало** prerouting-mikwg-правил
(см. 5.8): скрипт на каждом импорте пересобирает все правила `mikwg:` в
детерминированном порядке, поэтому `place-before` больше не используется.
Дополнительно добавлен MSS-clamp (`change-mss new-mss=1360` для
`to_vpn_mark`-SYN).

**Проверено после исправлений:** `last-handshake` обновляется; `ping 10.8.2.1`
(внутренний адрес сервера) — 3/3, ~5 мс; `tool fetch http://1.0.0.1/` через
туннель — HTTP 200, 55 KiB; с LAN-клиента через туннель внешний IP = `46.8.178.7`
(адрес VPN-сервера) вместо прямого `195.178.4.141`, HTTPS 200, скорость
~112 Мбит/с; peer `rx`/`tx` растут, конвертер без дропов.
ICMP на внешние адреса через туннель не проходит (типично для VPN-серверов), но
TCP работает — это и есть критерий. Также исправлена мёртвая input-rule
(`dst-port=51820` → реальный listen-port WG `13231`).

Побочно: `188.40.167.81` (2ip.io) не отвечает на ICMP даже напрямую — все
прошлые «timeout через туннель» по этому адресу были ложной целью теста.

Осталось: подтвердить работу с LAN-клиента и пройти Этап 6 (приёмка).

### Что развёрнуто (идемпотентным импортом `routeros.generated.rsc`)

- `veth-awg` 10.99.0.2/30 → `br-awg` 10.99.0.1/30; NAT masquerade
  `10.99.0.0/30 → out-interface-list=WAN`; input-accept udp/51820 in `br-awg`;
- mount `awg-cfg` (`list=awg-cfg`, `src=/usb1/awg-config`, `dst=/etc/awg`,
  `mode=ro`);
- контейнер `awg-converter`: `file=awg-converter-arm64.tar`, `interface=veth-awg`,
  `mountlists=awg-cfg`, `entrypoint=/awg-converter`, `dns=1.1.1.1`,
  `start-on-boot=yes`, `logging=yes` → **status running**; в логе — строка
  параметров обфускации без секретов;
- WireGuard `awg`: listen-port 13231, mtu 1408, адрес 10.8.2.5/32; peer
  `endpoint-address=10.99.0.2 endpoint-port=51820 allowed-address=0.0.0.0/0
  persistent-keepalive=12s` → **`last-handshake` обновляется**;
- антилуп-маршрут `{VPN_SERVER_IP}/32 via ether1` (main);
- `to_vpn_table`: `0.0.0.0/0 → awg`;
- mangle (комментарии `mikwg: ...`): anti-loop accept для IP сервера →
  mark-connection `to_vpn_mark` по `to_vpn_list` (prerouting, new) →
  mark-routing `to_vpn_table` (prerouting, connection-mark) → mark-routing по
  `to_vpn_list` (output);
- FastTrack уже имел `connection-mark=!to_vpn_mark` (наш `rscgen` это
  поддерживает идемпотентно);
- watchdog `awg-watchdog` (script + scheduler 1m).

Бэкапы на роутере: `pre-mikwg.backup` (старый), `pre-awg.backup` (наш),
`pre-mikwg-export.rsc`. Локально в проекте (в `.gitignore`): `awg0.conf`,
`routeros.generated.rsc`.

### Работает / не работает

- ✅ WireGuard handshake с сервером AmneziaWG 3.1 через конвертер проходит;
- ✅ контейнер запускается, парсит `awg0.conf`, слушает 0.0.0.0:51820, upstream —
  IP сервера;
- ✅ маркировка/mangle/anti-loop/FastTrack/watchdog на месте, дефолт не тронут;
- ❌ **data-path не проходит:**
  - peer `tx` растёт (пакеты уходят), `rx` почти не растёт (~92 Б — только
    handshake response);
  - из LAN `nc -z 188.40.167.81 443` (адрес из `to_vpn_list`) — **timeout**;
    `nc -z 1.1.1.1 443` (не в списке, напрямую) — OK;
  - с роутера `/ping 188.40.167.81` — timeout; временное добавление `1.1.1.1` в
    `to_vpn_list` → тоже timeout; `/ping 8.8.8.8` напрямую — OK.

Вывод: исходящий обфусцированный transport уходит к серверу, но ответные
data-пакеты не приходят/не принимаются. Handshake (init/resp) при этом работает
целиком, т.е. S/H/HP-ключ и домен MAC корректны — проблема специфична для
transport (data).

> **Текущее влияние на сеть (важно):** вся маркировка сейчас **активна**. Трафик
> к адресам из `to_vpn_list` уходит в нерабочий туннель и **не проходит**;
> остальной трафик (в т.ч. управление, дефолт) — напрямую и работает. Если
> неработающие адреса мешают до исправления — временно снимите mangle-правила
> `mikwg: mark connection` / `mikwg: route to vpn` / `mikwg: router via vpn`
> (или очистите `to_vpn_list`), сохранив остальное.

### Гипотезы для следующей сессии (по убыванию вероятности)

1. **Входящий transport отбрасывается** на `TransformIn` (счётчик
   `FromUpstreamDropped`) либо ядром после распаковки. Проверить verbose
   конвертера: `/container/set awg-converter cmd="-v"` + restart (env у
   конвертера нет, только флаги), затем смотреть лог `stats: ->upstream ...
   ->router ... dropped router/upstream ...` при пинге через туннель. (В этой
   сессии `cmd="-v"` мог уже выставиться — проверить `/container/print detail`.)
2. **Домен HeaderProtection для transport** (наш код XOR-ит ровно 16 Б — как
   `RoutineEncryption` в `amneziawg-go/device/send.go`; nonce = первые 12 Б S).
   Сверить не только с исходником, но и с реальным pcap сервера/эталонного
   клиента.
3. **ContentPaddingAddition исходяще** — сервер может ожидать content-padding в
   исходящем transport; проверить эталонным подключением/запросить конфиг без CPA.
4. **Нет настоящего интероп-стенда** (Этап 3 закрыт только in-process тестом):
   поднять amneziawg-go сервер+stock-клиент с теми же S/H/HPK/Jc/I1, снять pcap —
   получить «золотую» форму transport и сравнить.
5. **Сервер не форвардит `0.0.0.0/0`** для клиента (серверный AllowedIPs/NAT):
   handshake это не доказывает.

### Известные особенности RouterOS 7.24.5 (учтено в `rscgen`)

- `/container/mounts/add list=NAME src=... dst=... mode=ro` — свойства `name` и
  `read-only` не существуют; проверка — `find where list=NAME`;
- `/container/add mountlists=... entrypoint=... dns=...` — `mounts=` не
  существует; без явного `dns=` контейнер **не стартует**, если в `/ip/dns` нет
  статических серверов (у нас DoH-only);
- `/interface/wireguard/peers/add allowed-address=...` — не `allowed-ips`;
- `/ping` не принимает `routing-table=` (проверка маршрута — пингом адреса из
  `to_vpn_list` или временным добавлением контрольного IP);
- образ контейнера — **только с несжатым слоем** (`imagetool` обновлён);
- `/container/get ... status` не работает (свойство недоступно) — старт делаем
  через `:do { /container/start ... } on-error={}`.

### Как откатить (если понадобится)

Удалить mangle с комментариями `mikwg:`, маршруты `mikwg vpn` и
`mikwg server via WAN`, script+scheduler `awg-watchdog`, контейнер
`awg-converter`, интерфейс `awg` и адрес 10.8.2.5/32, mount `awg-cfg`,
`veth-awg`/`br-awg`/адрес 10.99.0.1/30, NAT `mikwg converter snat`, filter
`mikwg converter in`. `to_vpn_list`/`to_vpn_table` и дефолт остаются как были.
Rollback-скрипт пока не генерируется.

---

## 0. Контекст и цель

**Цель:** MikroTik c RouterOS 7.24+ (ARM64) должен стать полноценным клиентом
AmneziaWG 3.1, при этом:

- шифрование WireGuard остаётся в ядре RouterOS (быстро);
- приватный WG-ключ хранится только в RouterOS и в контейнер не попадает;
- контейнер (наш Go-бинарник `awg-converter`) только «переодевает» пакеты WG
  в формат AmneziaWG 3.1 на проводе и обратно.

**Почему это возможно:** AmneziaWG (линия 3.0/3.1, тег `v3.1.20260812`) не меняет
криптографию WireGuard. Все его механизмы — внешние слои над пакетом:

- `S1–S4` — мусорный префикс перед сообщением соответствующего типа;
- `H1–H4` — uint32-идентификатор вместо 4-байтного заголовка типа WG
  (диапазоны; в нашем конфиге вырождены до стандартных значений 1..4);
- `HeaderProtectionKey` — ChaCha20-шифрование заголовка, nonce = первые
  12 байт S-паддинга (поэтому S ≥ 12 — наш конфиг 129/106/23/12 удовлетворяет);
- `Jc/Jmin/Jmax`, `I1–I5` (CPS) — пакеты-приманки (односторонние, сервер игнорирует);
- `ContentPaddingAddition` — паддинг внутри шифротекста (односторонний, опциональный);
- таймеры с диапазонами — тайминговая обфускация (односторонняя).

Получатель срезает эти слои и валидирует MAC/AEAD по каноническому WG-сообщению
(проверить домен MAC1 — см. Этап 1; при худшем раскладе MAC1 пересчитывается
конвертером от публичного ключа сервера, секрета не требуется).

## 1. Исходные данные

> Реальный конфиг **получен с роутера 2026-10-03**; значения ниже совпадают с
> ним. Секреты (PrivateKey, HeaderProtectionKey, PublicKey) в репозиторий **не**
> попадают — они хранятся на роутере (`usb1/awg-config/awg0.conf`) и локально
> вне git. Endpoint — `{VPN_SERVER_IP}:51822`.

Рабочий конфиг: `usb1/awg-config/awg0.conf` на роутере (Amnezia VPN, сервер
`{VPN_SERVER_IP}:51822`, AmneziaWG-3.1 «mikrotik-msk-awg - kit»):

| Параметр | Значение | Действие конвертера |
|---|---|---|
| PrivateKey | {…} | НЕ используется конвертером; вводится в `/interface/wireguard` |
| Address | 10.8.2.5/32 | адрес на wg-интерфейсе RouterOS |
| DNS | 1.1.1.1, 8.8.8.8 | `/ip/dns` роутера (запросы уходят через туннель) |
| MTU | 1408 | mtu wg-интерфейса; outer = 1408+32+12+28 = 1480 ≤ 1500 ✓ |
| Jc/Jmin/Jmax | 3 / 56 / 188 | 3 мусорных пакета 56–188 Б перед initiation |
| S1/S2/S3/S4 | 129 / 106 / 23 / 12 | префикс перед init/resp/cookie/data |
| H1–H4 | 1 / 2 / 3 / 4 | идентификатор типа = стандартное значение WG |
| I1 | `<r 226>` | CPS-пакет: 226 случайных байт |
| HeaderProtectionKey | {…} | ChaCha20-шифрование заголовков (ключ маскировки) |
| ContentPaddingAddition | 18–36 | исходяще не воспроизводим — не требуется (односторонний) |
| RekeyAfterTime | 115–141 | kernel-WG rekey 120s ∈ диапазон ✓ (бесплатно) |
| RekeyTimeout | 5–8 | kernel-WG retry 5s ∈ диапазон ✓ |
| RejectAfterTime | 187–251 | поведение ядра, не трогаем |
| KeepaliveTimeout | 10–14 | `persistent-keepalive=12s` (+ опц. джиттер) |
| MaxHandshakeAttempts | 22–34 | поведение ядра (18 попыток/90с) — расхождение не критично |
| AllowedIPs | 0.0.0.0/0, ::/0 | маршруты; IPv6-решение — см. Этап 5, шаг 7 |

Железо: RB5009 / hAP ax³ (ARM64) — container package поддерживается.
Доставка образа: tar через Files/Winbox. Перезагрузки и короткий простой допустимы.

## 2. Архитектура

```
[LAN] → маршруты → wg-awg (kernel WG; private-key только здесь; mtu 1408)
                      │ peer endpoint = 10.99.0.2:51820 (veth, локально)
                      ▼
            ┌────────────────────────────────────┐
            │ container: awg-converter (Go)      │
            │  · слушает :51820                  │
            │  · S/H/HeaderProtection(Junk/CPS)  │
            │  · UPSTREAM = {VPN_SERVER_IP}:51822│
            │  · парсит смонтированный awg0.conf │
            └───────────┬────────────────────────┘
                        │ veth-awg → br-awg (10.99.0.1/30) → forward + masquerade → WAN
                        ▼
              [AmneziaWG 3.1 сервер :51822]
```

Потоки:
- **Outbound**: kernel WG формирует канонический WG-пакет → UDP на 10.99.0.2:51820 →
  конвертер классифицирует тип (байт 0), строит AWG-пакет
  (`[S-паддинг][защищённый заголовок][тело WG]`), перед initiation дополнительно
  шлёт Jc мусорных пакетов и CPS I1 → отправляет на UPSTREAM.
- **Inbound**: датаграмма от сервера → конвертер находит H-идентификатор по смещению
  S соответствующего типа, снимает шифрование заголовка и S-префикс, восстанавливает
  канонический WG-пакет → ядру роутера. Неопознанные пакеты молча отбрасываются.
- **Антилуп**: контейнер ходит к серверу через роутер — на роутере обязательно
  `/ip route add dst-address={VPN_SERVER_IP}/32 gateway={WAN_GW}` (иначе трафик
  контейнера уйдёт в туннель и зациклится).

## 3. Структура репозитория (целевая)

```
mikwg/
├── README.md                  # пользовательская документация (из README-draft.md)
├── PLAN.md                    # этот план
├── LICENSE                    # GPLv3 (уже есть)
├── go.mod                     # module github.com/nskforward/mikwg
├── Makefile                   # build / test / lint / image / tar / bench
├── .gitignore
├── cmd/
│   ├── awg-converter/main.go  # точка входа конвертера
│   └── rscgen/main.go         # генератор routeros.generated.rsc из awg0.conf
├── internal/
│   ├── config/                # парсер awg0.conf + env; валидация
│   ├── awg/
│   │   ├── params.go          # типы параметров (u16Range и т.п.)
│   │   ├── transform_out.go   # WG→AWG (типы 1/2/3/4)
│   │   ├── transform_in.go    # AWG→WG
│   │   ├── headercrypt.go     # ChaCha20 HeaderProtectionKey
│   │   ├── junk.go            # Jc/Jmin/Jmax
│   │   ├── cps.go             # парсер тегов I1–I5: <b>/<r>/<rc>/<rd>/<t>/<c>
│   │   └── mac1.go            # (fallback) пересчёт MAC1 от публичного ключа
│   └── proxy/
│       ├── proxy.go           # UDP-насос, обе стороны
│       └── jitter.go          # опциональная тайминговая обфускация
├── Dockerfile                 # multi-stage, CGO_ENABLED=0, arm64
├── build.sh                   # buildx --platform linux/arm64 + docker save
└── testbench/                 # локальный стенд (docker-compose)
    ├── docker-compose.yml
    ├── server/Dockerfile      # amnezia-awg-go как сервер (те же параметры)
    ├── client/Dockerfile      # stock wireguard-go как клиент
    └── run.sh
```

---

## Этап 0 — Подготовка репозитория (~0,5 ч) — ✅ выполнено

**Что делаем:**
1. `go mod init github.com/nskforward/mikwg`; каркас каталогов выше; `.gitignore`
   (`*.tar`, `testbench/pcap/`, `.env`).
2. `Makefile`: цели `test`, `lint` (`go vet` + `gofumpt`), `build`
   (`CGO_ENABLED=0 go build -trimpath -ldflags "-s -w"`), `image`, `tar`, `bench`.
3. Скопировать этот план в `PLAN.md`, черновик — в `README.md`.

**Результат:** пустой собирающийся каркас, `make build` даёт бинарник.

---

## Этап 1 — Спецификация трансформаций по исходникам (~3–5 ч) — ✅ выполнено

**Что делаем:** клонируем `github.com/amnezia-vpn/amnezia-awg-go` (основной
источник, Go) и `github.com/amnezia-vpn/amneziawg-linux-kernel-module`
(сверка поведения), читаем `device/noise*.go`, `device/receive.go`,
`device/send.go`, `device/uapi.go`. Закрываем чек-лист:

1. **Раскладка исходящего пакета каждого типа** при наших параметрах:
   где лежит S-паддинг, где H-идентификатор, что именно покрывает
   HeaderProtection (только заголовок после паддинга? весь пакет после паддинга?).
2. **Вариант ChaCha20**: RFC 8439 с nonce=первые 12 байт S-паддинга и counter=0,
   или XChaCha20, или chacha20poly1305; как используется HPK (32 байта напрямую
   или хешируется).
3. **Домен MAC1/MAC2**: по каким байтам считает получатель (каноническое
   сообщение без паддинга/шифрования — ожидаемо; если нет — включаем `mac1.go`,
   ключ MAC1 = BLAKE2s-128, выводится из публичного ключа сервера:
   `smac = MAC(HASH(LABEL_MAC1‖spub))`, `MAC1 = MAC16(smac, msg[:len-32])`).
4. **Junk-пакеты**: чисто случайные байты, длина ∈ [Jmin, Jmax], количество Jc,
   порядок и тайминг отправки относительно initiation (до/после CPS I1).
5. **CPS-теги**: точная грамматика `<b 0xHEX>`, `<r N>`, `<rc N>`, `<rd N>`, `<t>`,
   `<c>` (для нас фактически нужен только `<r 226>`, но парсер полный).
6. **Cookie (тип 3)**: раскладка с S3=23, поведение клиентской стороны
   (MAC2/cookie в kernel WG работает само — важно ничего не сломать на проходе).
7. **Входящий data с ContentPaddingAddition**: подтвердить, что «лишний» паддинг
   после расшифровки отрезается по IP total length (wireguard-linux делает
   `pskb_trim`; проверить, что реализация RouterOS наследует это поведение —
   эмпирически на Этапах 3/5).
8. **Порядок пакетов initiation-раунда**: [junk ×Jc] → [CPS I1] → [initiation]?
   и повторные ретрансмиссии (каждая ли с junk/CPS).

**Параллельно — эталонные pcap:** поднимаем на Mac пару
amnezia-awg-go-сервер + amnezia-awg-go-клиент с НАШИМИ параметрами (S/H/HPK/Jc/I1
из конфига), снимаем tcpdump: initiation+приманки, response, transport туда/обратно,
keepalive. Складываем в `testbench/pcap/*.pcap` — это золотые фикстуры для юнит-тестов
и эталон формы трафика для приёмки.

**Deliverables:** `docs/protocol-notes.md` с по-байтовыми схемами и ссылками на
файлы/строки исходников; pcap-фикстуры; закрытые все `TODO-Э1`.

**Критерий выхода:** по каждой из 8 позиций есть однозначный ответ с ссылкой на код
или зафиксированный pcap.

---

## Этап 2 — Конвертер `awg-converter` (~4–8 ч) — ✅ выполнено

**Конфигурация.** Источник — смонтированный `awg0.conf` (монтируем
`usb1/awg-config` read-only; `PrivateKey` парсер видит, но **не читает и не
логирует**). Переопределения env: `LISTEN` (по умолч. `0.0.0.0:51820`),
`UPSTREAM` (обязателен, если нет Endpoint в conf), `JITTER` (`off` по умолч.),
`LOG_LEVEL`. Валидация: `S≥12` при заданном HPK, `Jmax≥Jmin`, диапазоны таймеров
парсятся, но используются только для джиттера.

**Ядро — `internal/awg`:**
- `TransformOut(pkt []byte) (out [][]byte)`:
  - тип 1 (initiation): генерирует `[junk×3]`, `[CPS <r 226>]`, `[S1=129-префикс
    (первые 12 байт = nonce) + защищённый заголовок + тело initiation]`;
  - тип 2/3: `[S2|S3-префикс + защищённый заголовок + тело]`;
  - тип 4 (data): `[S4=12-префикс (nonce) + защищённый заголовок + тело WG]`;
  - ContentPaddingAddition исходяще НЕ добавляем (см. Этап 1.7).
- `TransformIn(pkt []byte) (canonical []byte, ok bool)`:
  - пробуем смещения S4/S2/S3 (от сервера приходят data/resp/cookie): читаем
    uint32 на смещении S, сверяем с {H4,H2,H3}; при совпадении — снимаем
    шифрование заголовка, срезаем префикс, возвращаем канонический WG-пакет;
  - иначе пакет отбрасывается (мусор/приманки с сервера не приходят).
- `headercrypt.go`: ChaCha20 по спецификации Этапа 1.2.
- Аллокации: пул буферов (`sync.Pool`), ноль копий где возможно; макс. датаграмма
  4096 Б.

**Прокси — `internal/proxy`:** два направления (router⇄upstream), UDP-сокет,
одна горутина-читатель на направление; «адрес роутера» динамический (roaming-safe:
последний источник). `SO_RCVBUF` поднят. Счётчики пакетов/байт/дропов, лог-строка
раз в 60 с. SIGTERM → чистый exit 0 (RouterOS останавливает контейнер).

**Производительность:** цель ≥ 300 Мбит/с на RB5009 (~62k pps при 600-байтовых
пакетах — для Go single-goroutine UDP relay достижимо с запасом). Если бенчмарк
`make bench` покажет меньше — варианты: несколько воркеров-писателей, `WriteMsgUDP`,
отключение джиттера. Решение принимаем по цифрам.

**Тесты:**
- golden-тесты против pcap-фикстур Этапа 1 (по-байтовое сравнение трансформаций
  для всех типов, включая вариативность через запечатанные PRNG-сиды);
- property-тест: `TransformIn(TransformOut(x)) == x` для всех типов;
- fuzz (native go fuzzing) на `TransformIn` (никаких паник на мусоре);
- fuzz на парсер CPS-тегов.

**Критерий выхода:** `make test` зелёный; кросс-сборка
`CGO_ENABLED=0 GOOS=linux GOARCH=arm64` собирается; бенчмарк трансформации
≥ 100k pps на dev-машине.

---

## Этап 3 — Локальный стенд на Mac (~2–4 ч) — 🟡 частично (заменён in-process тестом)

**`testbench/docker-compose.yml`:**
- `awg-server` — контейнер: сборка amnezia-awg-go из исходников, режим сервера,
  параметры 1:1 из рабочего конфига (S/H/HPK/Jc/I1), тестовый серверный ключ,
  `NET_ADMIN`, `/dev/net/tun`, включён `ContentPaddingAddition=18-36`;
- `awg-converter` — наш бинарник (amd64 для стенда), UPSTREAM на `awg-server:51822`;
- `wg-client` — **stock** `wireguard-go` + `wg-quick`: endpoint =
  `awg-converter:51820`, приватный ключ клиента, allowed-ips 0.0.0.0/0.
  Stock-клиент — принципиально: эмулирует роль kernel WG роутера.

**Тест-матрица (`testbench/run.sh`, результаты в `testbench/report/`):**

| # | Сценарий | Ожидание |
|---|---|---|
| 1 | Рукопожатие | peer established ≤ 5 с; ping 10.8.x.x через туннель |
| 2 | Данные | curl через туннель, без потерь |
| 3 | Rekey | через ~120 с — новое initiation (с junk+CPS), сессия не рвётся |
| 4 | Keepalive | 5 мин простоя — туннель жив, пустые data-пакеты ходят |
| 5 | pcap outbound | перед initiation видны 3 junk (56–188 Б) + CPS 226 Б; data-пакеты начинаются с 12-байтового префикса, стандартной WG-подписи `01 00 00 00` в начале датаграммы нет |
| 6 | pcap inbound | сервер→клиент: data с ContentPadding 18–36 Б принимаются stock-клиентом (лишнее отрезается по IP tot_len) — **валидация гипотезы Этапа 1.7** |
| 7 | Мусор на вход конвертера | случайные датаграммы в listener — молча дропаются, прокси жив |
| 8 | Throughput | iperf3 client↔server через конвертер, фиксируем базу |

**Критерий выхода:** 8/8 сценариев зелёные; pcap конвертера структурно совпадает с
эталонным pcap amnezia-awg-клиента (те же слои/размеры).

---

## Этап 4 — Образ и генератор .rsc (~1 ч) — ✅ выполнено

**Dockerfile (multi-stage):** `golang:1.23-alpine` → сборка статики
(`CGO_ENABLED=0`), финал `alpine:3.20` (busybox для отладки) + `/awg-converter`
+ `USER nobody` + `ENTRYPOINT ["/awg-converter"]`. Никаких томов на запись.

**build.sh:** `docker buildx build --platform linux/arm64 -t mikwg/awg-converter:1.0.0
--load . && docker save … -o awg-converter-arm64.tar`. Проверка: `docker run
--platform linux/arm64 --rm mikwg/awg-converter:1.0.0 --version` (эмуляция qemu).

**`cmd/rscgen`:** читает локальную копию `awg0.conf` и рендерит встроенный
идемпотентный шаблон `.rsc` (см. Этап 5), подставляя ключи, IP сервера и параметры
WAN — исключает ручные опечатки. Переиспользует `to_vpn_list`/`to_vpn_table`
(флаги `-addr-list`/`-rt-table`); `-wan-gw`, `-wan-iface-list`, `-tar`, `-wg-name`,
`-wg-port` — аргументы CLI. Скрипт **не содержит** `reset-configuration`.

**Критерий выхода:** tar ~15–25 МБ загружается в Files; `rscgen` выдаёт полный
скрипт без плейсхолдеров (кроме осознанно оставленных).

### Публикация в Docker Hub (2026-10-03) — ✅ выполнено

Образ публикуется как `docker.io/nskforward/mikwg`; установка идёт через
`/container/add remote-image=`, а не через загрузку tar в Files.

- **`cmd/imagetool`:** добавлен режим `-push` (флаги `-image` — репозиторий,
  `-tag`/`-tags` — теги, `-out` — одновременно писать tar). Образ собирается в
  двух видах: **несжатый** слой (`OCIUncompressedLayer`) — для оффлайн-tar
  (`file=`), и **gzip**-слой (`DockerLayer`) — для реестра. Это выяснилось
  эмпирически на роутере: `remote-image` тянет blob как `<digest>.tar.gzip` и
  всегда распаковывает его, поэтому несжатый слой падает с `download/extract
  error: extract layer failed`; при этом `file=` сжатый слой не принимает.
  `entrypoint`, `user 65534`, детерминированный `created`, OCI-метки. Пуш —
  через `remote.Write` (`go-containerregistry`), auth из `DOCKER_USERNAME`/
  `DOCKER_PASSWORD` или `docker login`; Docker-демон не нужен. Тесты
  `cmd/imagetool/main_test.go` проверяют оба вида и пуш в in-process
  OCI-registry — без Docker и без сети.
- **`cmd/rscgen`:** новый флаг `-image` (по умолчанию `nskforward/mikwg:1.0.1`)
  включает registry-режим: `/container/config/set registry-url=... tmpdir=...` +
  `/container/add remote-image=... root-dir=...`. Пустой `-image` сохраняет
  оффлайн-путь через `file=<tar>`. Флаги `-registry`, `-root-dir`, `-tmpdir`.
- **CI:** `.github/workflows/release.yml` на тег `v*` (и `workflow_dispatch`):
  `make lint test` → `make build` → `imagetool -push` тегов `X.Y.Z` и `latest` +
  `awg-converter-arm64.tar` → GitHub Release. Секреты `DOCKER_USERNAME`/
  `DOCKER_PASSWORD`.
- **Makefile:** `push` / `push-release`; `REGISTRY ?= docker.io/nskforward/mikwg`.

**Критерий выхода (обновлено):** образ `nskforward/mikwg` собирается и пушится без
Docker-демона; RouterOS 7.24 поднимает контейнер из `remote-image`; tar остаётся
оффлайн-опцией.

#### Проверка на роутере (2026-10-03)

Миграция прошла на hAP ax³ / RouterOS 7.24.5:
- `/container/config/set registry-url=https://registry-1.docker.io tmpdir=usb1/tmp`;
- старый контейнер (`file=`) удалён, добавлен
  `/container/add remote-image=nskforward/mikwg:1.0.1 interface=veth-awg
  root-dir=usb1/images/awg-converter mountlists=awg-cfg entrypoint=/awg-converter
  dns=1.1.1.1 start-on-boot=yes logging=yes name=awg-converter`;
- лог: `downloading and extracting ... arch=arm64` → `download/extract done`,
  контейнер `running`, `awg-converter 1.0.1`, handshake свежий;
- `fetch https://core.telegram.org/` (адрес из `to_vpn_list`) — HTTP 200.

**Найдено эмпирически:** `remote-image` тянет blob как `<digest>.tar.gzip` и
**всегда** распаковывает его — несжатый слой падает с `download/extract error:
extract layer failed`. Поэтому для реестра нужен gzip-слой (см. `imagetool`),
а несжатый — только для оффлайн `file=`. Релиз `1.0.1` исправляет это
(`application/vnd.docker.image.rootfs.diff.tar.gzip`).

---

## Этап 5 — Настройка RouterOS (~1–2 ч + приёмка) — 🟢 развёрнуто

> ⚠️ **mikwg не сбрасывает роутер.** Подготовка (бэкап, пакет `container`,
> `device-mode`, загрузка файлов) — на стороне пользователя
> ([`README.md`](README.md)). Перед началом обеспечьте гарантированный доступ по
> [`docs/safe-operations.md`](docs/safe-operations.md) (LAN-порт или
> Winbox-by-MAC; при необходимости — Safe Mode Ctrl-X). `run-after-reset` не
> требуется.

> Выполняется по шагам с контролем после каждого. Команды генерирует `rscgen` в
> `routeros.generated.rsc` — **идемпотентно, поверх рабочей конфигурации**;
> таблица/список `to_vpn_table`/`to_vpn_list` переиспользуются. Здесь — канва с
> пояснениями.

**5.0 Предварительно (пользователь):** `/system/backup/save name=pre-mikwg` +
`/export file=pre-mikwg-export`, **скачать оба файла на рабочую станцию**.
Проверить: `/system/resource/print` (free hdd ≥ 64 МБ), версия 7.24+.

**5.1 Container package (пользователь):** загрузить `container-7.24-arm64.npk`
(через `/tool/fetch` или Winbox) → перезагрузка → меню `/container` появилось.

**5.2 Включить режим контейнеров (пользователь):**
`/system/device-mode/update container=yes` → подтвердить → перезагрузка
(на части устройств требует физического подтверждения кнопкой).

**5.3 Сеть контейнера** (создаётся только при отсутствии):
```
/interface/veth/add name=veth-awg address=10.99.0.2/30 gateway=10.99.0.1
/interface/bridge/add name=br-awg
/interface/bridge/port/add bridge=br-awg interface=veth-awg
/ip/address/add address=10.99.0.1/30 interface=br-awg
/ip/firewall/filter/add chain=input action=accept protocol=udp dst-port=51820 \
    in-interface=br-awg comment="mikwg converter in"
/ip/firewall/nat/add chain=srcnat action=masquerade src-address=10.99.0.0/30 \
    out-interface-list=WAN comment="mikwg converter snat"
```

**5.4 Контейнер:** конфиг монтируется read-only; конвертер берёт `UPSTREAM` из
`Endpoint` смонтированного `awg0.conf`. **Синтаксис для RouterOS 7.24.5** (в
старых версиях иначе: `name=`/`read-only=yes`/`mounts=`; здесь —
`list=`/`mode=ro`/`mountlists=`). `dns=` обязателен, если в `/ip/dns` нет
статических серверов (DoH-only — как у нас). Образ тянется из Docker Hub
(`remote-image`); для оффлайн-режима — `file=awg-converter-arm64.tar`:
```
/container/config/set registry-url=https://registry-1.docker.io tmpdir=usb1/tmp
/container/mounts/add list=awg-cfg src=/usb1/awg-config dst=/etc/awg mode=ro
/container/add remote-image=nskforward/mikwg:1.0.1 interface=veth-awg \
    root-dir=usb1/images/awg-converter mountlists=awg-cfg \
    entrypoint=/awg-converter dns=1.1.1.1 start-on-boot=yes logging=yes name=awg-converter
/container/start awg-converter
```
> `remote-image` распаковывает слой при загрузке, поэтому в реестре образ обязан
> иметь gzip-слой (см. «Публикация в Docker Hub»); несжатый слой — только для
> `file=`-импорта оффлайн-tar.

Контроль: `/container/print` → running; лог — строка конфигурации без секретов.

**5.5 WireGuard (ключ только здесь):**
```
/interface/wireguard/add name=awg listen-port=13231 mtu=1408 private-key="{PRIVATE_KEY}"
/ip/address/add address=10.8.2.5/32 interface=awg
/interface/wireguard/peers/add interface=awg public-key="{PUBLIC_KEY}" \
    endpoint-address=10.99.0.2 endpoint-port=51820 persistent-keepalive=12s \
    allowed-address=0.0.0.0/0
```
> В RouterOS параметр называется `allowed-address`, **не** `allowed-ips`.
Контроль: в течение ~10 с `/interface/wireguard/peers/print` покажет
`last-handshake`. Если нет — см. Troubleshooting в README (S/HPK, логи контейнера).

**5.6 Антилуп-маршрут (обязательно до маркировки трафика):**
```
/ip/route/add dst-address={VPN_SERVER_IP}/32 gateway={WAN_GW} distance=1 comment="mikwg server via WAN"
```

**5.7 Выборочная маршрутизация — переиспользование `to_vpn_table`:**
`to_vpn_table` уже существует; таблица создаётся только если её нет, наш дефолт
добавляется один раз (по комментарию `mikwg vpn`):
```
:if ([:len [/routing/table find name=to_vpn_table]] = 0) do={
    /routing/table/add name=to_vpn_table fib
}
/ip/route/add dst-address=0.0.0.0/0 gateway=awg routing-table=to_vpn_table comment="mikwg vpn"
```
Проверка: `/ping` в RouterOS 7.24.5 **не принимает `routing-table=`**. Проверяйте
маршрут пингом адреса из `to_vpn_list` (он уйдёт через туннель) либо временно
добавьте контрольный IP в список. Если в `to_vpn_table` уже есть чужой дефолт —
разобраться **до** импорта (скрипт добавляет свой дефолт только при отсутствии
правила `mikwg vpn`).

**5.8 Mangle-маркировка (создаётся скриптом; порядок важен для производительности):**
```
# prerouting, в порядке применения:
/ip/firewall/mangle/add chain=prerouting in-interface=awg action=accept passthrough=no comment="mikwg: vpn return"
/ip/firewall/mangle/add chain=prerouting dst-address={VPN_SERVER_IP}/32 action=accept passthrough=no comment="mikwg: anti-loop"
/ip/firewall/mangle/add chain=prerouting connection-state=new dst-address-list=to_vpn_list action=mark-connection new-connection-mark=to_vpn_mark passthrough=yes comment="mikwg: mark connection"
/ip/firewall/mangle/add chain=prerouting connection-mark=to_vpn_mark action=mark-routing new-routing-mark=to_vpn_table passthrough=no comment="mikwg: route to vpn"
# output, трафик самого роутера:
/ip/firewall/mangle/add chain=output dst-address={VPN_SERVER_IP}/32 action=accept passthrough=no comment="mikwg: anti-loop out"
/ip/firewall/mangle/add chain=output connection-state=new dst-address-list=to_vpn_list action=mark-connection new-connection-mark=to_vpn_mark passthrough=yes comment="mikwg: mark connection out"
/ip/firewall/mangle/add chain=output connection-mark=to_vpn_mark action=mark-routing new-routing-mark=to_vpn_table passthrough=no comment="mikwg: router via vpn"
```
Скрипт **пересобирает** все правила с комментарием `mikwg:` на каждом импорте
(удалить + добавить заново), поэтому порядок детерминирован, а старая раскладка
мигрируется тем же импортом. `vpn return` обязан быть **первым** в prerouting —
это горячее (загрузочное) направление из туннеля, и такой порядок выводит его из
mangle сразу. Anti-loop — **выше** правил маркировки, иначе трафик до сервера
(если его IP попал в `to_vpn_list`) зациклится. Просмотр `to_vpn_list` выполняется
только для `connection-state=new`; установленные пакеты маршрутизируются по одной
`connection-mark`.

**FastTrack-ловушка:** FastTrack обходит `mangle` и ломает policy routing. Скрипт
идемпотентно выставляет `connection-mark=!to_vpn_mark` на правило(а)
`fasttrack-connection`; если у правила уже другой mark — пишет предупреждение в
лог, не перезаписывая. На текущем роутере FastTrack уже настроен как
`connection-mark=!to_vpn_mark` (хвост), т.е. менять нечего.

**5.9 Включение выборочной маршрутизации:** пополнить `to_vpn_list`
(`/ip/firewall/address-list/add list=to_vpn_list address=...`) и проверить с
LAN-клиента. Дефолтный маршрут роутера **не трогаем**; «всё через VPN» = добавить
`0.0.0.0/0` в список.

**5.10 DNS:** по умолчанию скрипт **не меняет** `/ip/dns` (строка
`/ip/dns/set ...` закомментирована). Если нужно, чтобы DNS роутера (1.1.1.1,
8.8.8.8) шёл через туннель — добавьте эти адреса в `to_vpn_list`; правило
`chain=output` уже сгенерировано.

**5.11 Решение по IPv6:** туннель IPv6 не несёт (в конфиге только IPv4-адрес).
Если провайдер даёт IPv6 — либо отключить на LAN
(`/ipv6/settings/set disable-ipv6=yes`), либо осознанно оставить прямой доступ:
v6-трафик просто не маршрутизируется в туннель (утечки «мимо VPN» в терминах
выборочной маршрутизации нет).

**5.12 Watchdog (генерируется, идемпотентно):** scheduler раз в 1 мин: если
`last-handshake` пира `awg` старше 5 мин → restart контейнера.

**Результат этапа:** туннель поднят; адреса из `to_vpn_list` ходят через
AmneziaWG 3.1; watchdog и start-on-boot на месте; `to_vpn_list`/`to_vpn_table`
переиспользованы; дефолтный маршрут не изменён. Генерация rollback-скрипта —
отдельной задачей (пока не реализована).

---

## Этап 6 — Приёмка (~1 ч активных + 24 ч наблюдения) — ⬜ не начато

- **Handshake/rekey:** сутки наблюдения: `last-handshake` обновляется каждые
  ~2 мин, перезапусков контейнера нет (`/log print where topics~"container"`).
- **Пропускная способность:** скачивание большого файла с LAN-клиента через
  туннель (например, `speed.hetzner.de` / Cloudflare speed), цель:
  ≥ 200–300 Мбит/с; CPU роутера при этом < 80% (`/system/resource/cpu/print`).
- **Форма трафика:** `/tool/sniffer/quick interface=WAN port=51822` — убедиться:
  датаграммы не начинаются с `01 00 00 00` (стандартный WG-заголовок), перед
  initiation идут мусорные пакеты и CPS 226 Б.
- **Маршрутизация:** адреса из `to_vpn_list` идут через туннель, остальное — напрямую;
  проверить, что anti-loop работает, а маркированные соединения не уходят в WAN
  напрямую в обход `to_vpn_table` (FastTrack-исключение).
- **Отказоустойчивость:** вынуть/вставить USB с конфигом не должно валить роутер;
  перезагрузка роутера → контейнер и туннель поднимаются сами.
- Зафиксировать фактические цифры в README (раздел Performance).

---

## Этап 7 — README и финализация (~1 ч) — ✅ выполнено

README актуализирован: установка и обновление из Docker Hub, раздел проверки,
устранение неполадок (в т.ч. требование gzip-слоя для `remote-image` и
несжатого — для оффлайн-tar), обновление параметров обфускации через
`awg0.conf` + `/container/restart`. Опубликованы теги `v1.0.0`, `v1.0.1`
(`latest` → `1.0.1`). Осталось: фактические цифры производительности на роутере
(Этап 6).

---

## Оптимизация hot path и файрвола (2026-10-03) — сделано, остаток F

Минимизирована работа с памятью на пути данных и упорядочена обработка пакетов
файрволом:

- `TransformIn`: точный пре-чек длины handshake (датаграммы init/resp/cookie
  имеют фиксированный размер `S+148/92/64`; остальное сразу в
  `unwrapTransport`) и расшифровка заголовка **in-place** в приёмном буфере — без
  `make+copy`;
- `WrapTransportInPlace`: исходящее кадрирование **in-place** в буфере с запасом
  `S4` слева — тело пакета не копируется, буфер не аллоцируется;
- паддинг S1–S4 из буферизованного `crypto/rand` (`internal/awg/rand.go`) — без
  getrandom-syscall на исходящий пакет;
- прокси: `ReadFromUDPAddrPort`/`WriteToUDPAddrPort`, адрес роутера как
  `netip.AddrPort`, обновляется только при смене (без аллокаций на пакет);
- `rscgen`: mangle пересобирается в фиксированном порядке на каждом импорте —
  `vpn return` первым (горячее направление), `anti-loop`, затем
  `connection-state=new` + `mark-connection` и дешёвая per-packet маркировка по
  `connection-mark`; отдельная пара правил в `output` + свой anti-loop.

Бенчмарки трансформаций (Apple M5 Pro, Go 1.26): inbound transport
795 ns/2944 B/5 allocs → 178 ns/384 B/1 alloc; outbound transport
497 ns/1944 B/3 allocs → 167 ns/384 B/1 alloc; initiation
251 ns/544 B/2 allocs → 202 ns/384 B/1 alloc.

**TODO (этап F):** снять последнюю аллокацию — `chacha20.Cipher` (384 B на
пакет). Собственная block-функция ChaCha20 с предвычисленным ключевым состоянием
(`HeaderProtectionKey` фиксирован, нужно ≤16 Б keystream), обязательны
golden-тесты против `golang.org/x/crypto/chacha20` на случайных nonce. Включать по
результатам замера на роутере (остаточный GC на целевой скорости).

---

## Конфигурация через `/container/envs` (v1.1.1, 2026-10-03) — ✅ выполнено

Параметры обфускации больше не передаются смонтированным `awg0.conf`, а
задаются environment-переменными контейнера — так их можно точечно менять прямо
в RouterOS/Winbox и применять через `/container/restart`.

- **`internal/config.ApplyEnviron`**: по-полевое переопределение из
  `KEY=VALUE`-окружения (имена регистронезависимы, синтаксис значений 1:1 с
  `awg0.conf`). Приоритет: флаги > env > файл > дефолты.
- **`cmd/awg-converter`**: `-conf` опционален (отсутствие файла — не ошибка);
  `LISTEN`/`JITTER`/`VERBOSE` читаются из env; в лог добавлен источник конфига
  (`config source: env` / `file` / `file+env`).
- **`cmd/rscgen`**: по умолчанию генерирует список `/container/envs` `awg-env`
  (UPSTREAM, S1–S4, H1–H4, JC/JMIN/JMAX, HPK, I1–I5) и вешает `envlists=awg-env` на
  контейнер; mount `awg0.conf` не создаётся. При импорте в существующий
  контейнер скрипт сначала при необходимости обновляет `remote-image` +
  `/container/update`, затем переключает контейнер на env-режим и снимает
  старый mount. Флаг `-conf-mount` сохраняет прежний файловый режим, `-env-list`
  переопределяет имя списка.
- **Безопасность**: `HPK` теперь виден в `/container/envs/print` и `/export`
  (это ключ обфускации, а не аутентификации; `PrivateKey` по-прежнему только в
  `/interface/wireguard`). Предупреждение добавлено в README.
- **Тесты**: `internal/config` (переопределение, env-only == файл, ошибки с
  именем переменной), `cmd/rscgen` (env-режим, legacy mount-режим, `buildEnvs`,
  экранирование значений).
- **Правка значения**: `/container/envs/set [find where list=awg-env && key="S4"] value=16`
  + `/container/restart awg-converter` (в Winbox — Container → Envs).

---

## Риски и митигации

| Риск | Вероятность | Митигация |
|---|---|---|
| Домен MAC1 в 3.x считается по «сырым» байтам | средняя | `internal/awg/mac1.go`: пересчёт MAC1 от публичного ключа сервера (секрета не требует); закрывается на Этапе 1–2 |
| Неточность домена HeaderProtection | средняя | golden-тесты по pcap реального amnezia-awg-клиента; Этап 1.2 обязателен до кода |
| RouterOS-WG не режет входящий ContentPadding | низкая | проверяется на Этапе 3.6 (stock wireguard-go режет) и Этапе 5; при провале — запросить у Amnezia конфиг без CPA (он опционален) |
| Пропускная способность < цели | низкая | оптимизация пула буферов/воркеров; предельный вариант — fallback B (полный amnezia-awg-go в контейнере) как план Б |
| device-mode требует физической кнопки | низкая | выполняется однократно по подсказке RouterOS |
| FastTrack обходит mangle и ломает policy routing | средняя | скрипт идемпотентно выставляет `connection-mark=!to_vpn_mark` на правило(а) FastTrack (на текущем роутере уже так); проверяется на Этапах 5.8/6 |
| В `to_vpn_table` уже есть чужой дефолт/маршруты | средняя | инвентаризация (Точка возобновления, шаг 1) до импорта; скрипт добавляет свой дефолт только при отсутствии правила с комментарием `mikwg vpn` |
| «Хвосты» (`to_vpn_list`/`to_vpn_table`) в неожиданном состоянии | низкая | имена переопределяются флагами `-addr-list`/`-rt-table`; инвентаризация перед импортом |
| Ротация конфига провайдером | средняя | инструкция в README + rscgen; параметры живут в смонтированном файле, пересборка образа не нужна |

## Таймлайн (суммарно ~1,5–2 рабочих дня)

| Этап | Часы | Зависимость |
|---|---|---|
| 0 каркас | 0,5 | — |
| 1 спецификация + pcap | 3–5 | — |
| 2 конвертер + тесты | 4–8 | 1 |
| 3 стенд | 2–4 | 2 |
| 4 образ + rscgen | 1 | 2 |
| 5 RouterOS | 1–2 | 3, 4 |
| 6 приёмка | 1 + 24ч наблюдения | 5 |
| 7 README | 1 | 6 |

## Definition of Done

1. Трафик к адресам из `to_vpn_list` у LAN-клиентов идёт через AmneziaWG 3.1
   (проверено по внешнему IP и pcap-форме трафика), остальное — напрямую.
   Дефолтный маршрут роутера не менялся.
2. Приватный WG-ключ существует только в RouterOS.
3. ≥ 200 Мбит/с через туннель (hAP ax³ / RB5009) при CPU < 80%.
4. 24 часа без деградации, rekey каждые ~2 мин, автостарт после перезагрузки.
5. `make test` зелёный; README воспроизводим для «такого же» роутера без factory
   reset (подготовка — вручную пользователем).
