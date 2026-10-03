# mikwg — AmneziaWG 3.1 на MikroTik RouterOS

[![release](https://img.shields.io/github/v/release/nskforward/mikwg.svg)](https://github.com/nskforward/mikwg/releases/latest)

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

- MikroTik ARM64 (проверялось на RB5009 и hAP ax³, RouterOS 7.24.5), RouterOS 7.24+
- **Рабочая конфигурация роутера** — mikwg не выполняет factory reset и
  не заменяет вашу конфигурацию, а дополняет её
- Пакет `container` той же версии RouterOS
- Внешний носитель (USB/SATA), смонтированный в RouterOS: на нём лежит образ
  контейнера (по умолчанию `usb1/images/awg-converter`, staging `usb1/tmp`). Для
  `remote-image` это важно: загрузка требует места больше, чем доступно во flash.
  Сам `awg0.conf` на роутере **не нужен** — параметры обфускации передаются в
  контейнер как environment-переменные (см. ниже)
- ≥ 64 МБ свободного места во flash (сам образ кладётся на носитель)
- Конфиг AmneziaWG 3.1 (поля `S1-S4`, `H1-H4`, `HeaderProtectionKey`, `Jc/Jmin/Jmax`, `I1-I5`)
- Для выборочной маршрутизации — address-list `to_vpn_list` (создаётся заранее
  или переиспользуется, если уже существует)
- Готовый бинарник `rscgen` из релиза — для установки; Go 1.26+ — только для
  сборки из исходников (Docker — опционально)
- Доступ роутера в интернет для загрузки образа из Docker Hub (для
  оффлайн-установки из tar не требуется)

## Быстрый старт

Установка состоит из двух частей: сначала на компьютере вы готовите скрипт
установки, затем один раз импортируете его на роутер. Ниже — по шагам.

### Шаг 0. Что понадобится

- **Роутер MikroTik на ARM64** (например, RB5009 или hAP ax³) с RouterOS 7.24
  или новее.
- **Доступ к роутеру** через Winbox (или SSH/Terminal). Заранее убедитесь, что
  изменения не отрежут вам доступ к управлению (подключайтесь по LAN-порту или
  Winbox-by-MAC).
- **Конфиг AmneziaWG** — файл `awg0.conf`.
- **Компьютер** (Windows, macOS или Linux) — на нём генерируется скрипт.
- **USB-флешка или диск**, подключённый к роутеру: образ контейнера не помещается
  во встроенную память. Носитель будет виден в RouterOS как `usb1` (или `disk1`).
- **Интернет на роутере** — образ скачивается из Docker Hub самим роутером.

Номер последней версии всегда можно посмотреть в бейдже в начале этой страницы
или на [странице релизов](https://github.com/nskforward/mikwg/releases/latest).

### Шаг 1. Возьмите конфиг AmneziaWG (`awg0.conf`)

Файл должен содержать параметры обфускации (`S1`–`S4`, `H1`–`H4`,
`HeaderProtectionKey`, `Jc/Jmin/Jmax`, `I1`–`I5`) и адрес сервера (`Endpoint`).
Если файла нет — возьмите его у VPN-провайдера или экспортируйте из клиента
AmneziaWG.

### Шаг 2. Скачайте генератор `rscgen`

Генератор запускается на компьютере и превращает `awg0.conf` в скрипт для
роутера. Откройте
[страницу последнего релиза](https://github.com/nskforward/mikwg/releases/latest),
в разделе **Assets** скачайте файл под вашу систему:

| Ваша система | Файл |
|---|---|
| macOS (Apple Silicon, M1–M4) | `rscgen-<версия>-darwin-arm64` |
| macOS (Intel) | `rscgen-<версия>-darwin-amd64` |
| Linux | `rscgen-<версия>-linux-amd64` |
| Windows | `rscgen-<версия>-windows-amd64.exe` |

На macOS и Linux сделайте его исполняемым (в терминале, в папке с файлом):
```bash
chmod +x rscgen-<версия>-darwin-arm64
```

> Если у вас установлен Go 1.26+, скачивать ничего не нужно — вместо имени файла
> используйте `go run github.com/nskforward/mikwg/cmd/rscgen@latest`.

### Шаг 3. Сгенерируйте скрипт установки

Положите `awg0.conf` и скачанный `rscgen` в одну папку и выполните из неё:

```bash
# macOS / Linux
./rscgen-<версия>-darwin-arm64 -conf awg0.conf -out routeros.generated.rsc
```
```powershell
# Windows (PowerShell)
.\rscgen-<версия>-windows-amd64.exe -conf awg0.conf -out routeros.generated.rsc
```

Рядом появится файл **`routeros.generated.rsc`** — это и есть скрипт для
роутера. По умолчанию в него подставляется образ `nskforward/mikwg:latest`
(самый свежий релиз); зафиксировать конкретную версию можно флагом
`-image nskforward/mikwg:X.Y.Z`.

> 🔒 Файл `*.generated.rsc` содержит **приватный ключ WireGuard** — не коммитьте
> его в git (он уже в `.gitignore`) и не передавайте третьим лицам.

> ⚠️ Антилуп-маршрут до IP сервера должен указывать на **реальный next-hop**
> (`gateway=192.168.1.1`), а не на интерфейс (`gateway=ether1`). Interface-route
> заставляет RouterOS ARP-ить публичный IP прямо в LAN-сегменте WAN, что на
> обычном ethernet-аплинке не срабатывает и полностью ломает туннель.
> `rscgen` определяет next-hop автоматически; переопределить можно флагом
> `-wan-gw` (именно IP шлюза, не интерфейс).

### Шаг 4. Подготовьте роутер (один раз)

> ⚠️ mikwg **никогда не сбрасывает роутер**: ни `rscgen`, ни сгенерированный
> скрипт не содержат `reset-configuration`. Установка идёт поверх вашей рабочей
> конфигурации. Перед началом прочитайте
> [`docs/safe-operations.md`](docs/safe-operations.md).

1. **Сделайте и скачайте бэкап.** В терминале роутера:
   ```rsc
   /system/backup/save name=pre-mikwg
   /export file=pre-mikwg-export
   ```
   Затем в Winbox откройте **Files**, выделите `pre-mikwg.backup` и
   `pre-mikwg-export.rsc` и перетащите их на рабочий стол компьютера кнопкой
   **Download** (или Drag&drop).
2. **Установите пакет `container`.** Откройте
   [mikrotik.com/download](https://mikrotik.com/download), выберите свою модель
   роутера и **ту же версию RouterOS**, что стоит на роутере, затем в разделе
   **Extra packages** скачайте файл `container-<версия>-arm64.npk`. В Winbox
   откройте **Files** и перетащите `.npk` в список (**Upload**), затем
   перезагрузите роутер: **System → Reboot**.
   > Версия пакета обязана совпадать с версией RouterOS, иначе роутер может не
   > загрузиться.
3. **Включите режим контейнеров.** В терминале:
   ```rsc
   /system/device-mode/update container=yes
   ```
   RouterOS попросит подтвердить действие нажатием кнопки на корпусе роутера
   (или перезагрузкой) — это защита от случайного включения контейнеров.
4. **Подключите USB-носитель.** Вставьте флешку/диск в роутер и проверьте, что
   он виден:
   ```rsc
   /file/print
   ```
   Носитель появится как `usb1` (или `disk1`). Если его нет — отформатируйте
   носитель в FAT32/ext4 и подключите снова.

### Шаг 5. Загрузите скрипт на роутер и импортируйте его

1. В Winbox откройте **Files** и перетащите туда `routeros.generated.rsc`
   (**Upload**).
2. Откройте **New Terminal** и выполните:
   ```rsc
   /import file=routeros.generated.rsc
   ```
3. Скрипт создаст контейнер и всё окружение, скачает образ `nskforward/mikwg`
   из Docker Hub и запустит его. Первая загрузка образа занимает несколько
   секунд (в логе видны стадии download/extract).

Скрипт **идемпотентен** (повторный импорт безопасен) и делает следующее:

- создаёт veth/bridge/NAT для контейнера и список `/container/envs` `awg-env` с
  параметрами обфускации (`awg0.conf` на роутер копировать не нужно — он не
  монтируется);
- настраивает `/container/config` (`registry-url`, `tmpdir`) и по `remote-image`
  скачивает образ `nskforward/mikwg` из Docker Hub, затем запускает контейнер
  `awg-converter` с `envlists=awg-env` (при оффлайн-режиме — импортирует локальный
  tar). Если контейнер уже существует и тег образа отличается, скрипт сам
  переключит `remote-image` и выполнит `/container/update`;
- поднимает WireGuard с endpoint на контейнер (приватный ключ остаётся в RouterOS);
- добавляет антилуп-маршрут до IP сервера через реальный WAN next-hop;
- добавляет srcnat в туннель (`out-interface=awg`), чтобы трафик роутера и
  LAN-клиентов уходил к серверу с внутренним адресом клиента (см. следующий раздел);
- **переиспользует** существующие `to_vpn_list` и `to_vpn_table` для выборочной
  маршрутизации (см. следующий раздел);
- ставит watchdog и `start-on-boot`.

**Дефолтный маршрут роутера не меняется.**

### Шаг 6. Проверьте, что всё работает

Выполните в терминале роутера:
```rsc
# контейнер должен быть в статусе running
/container/print
# параметры обфускации, переданные в контейнер (S/H/HPK/Jc/I1)
/container/envs/print where list=awg-env
# рукопожатие с сервером установлено: last-handshake должен обновляться
/interface/wireguard/peers/print
# трафик до адреса из to_vpn_list уходит через туннель (команда должна отработать без ошибок)
/tool/fetch url=https://core.telegram.org/ output=none
```

В логе контейнера видно строку `mikwg awg-converter <версия>`, источник конфига
(`config source: env`) и распознанные параметры обфускации (без секретов):
`/log/print where message~"awg-converter"` или
`/log/print where topics~"container"`.

> ICMP до внутреннего адреса сервера (`ping 10.8.2.1`) на многих VPN-серверах не
> проходит — это нормально. Критерий работоспособности — TCP-трафик через
> туннель (`fetch`, браузер с LAN-клиента).

### Продвинутые детали: флаги, образ, оффлайн-установка

**Флаги `rscgen`** (все необязательные; значения по умолчанию подходят для
большинства):

| Флаг | По умолчанию | Назначение |
|---|---|---|
| `-conf` | `awg0.conf` | путь к реальному конфигу AmneziaWG |
| `-out` | `routeros.generated.rsc` | имя выходного скрипта |
| `-image` | `nskforward/mikwg:latest` | образ из Docker Hub; пустая строка + `-tar` включает оффлайн-режим |
| `-tar` | `awg-converter-arm64.tar` | имя tar-файла на роутере (для оффлайн-режима) |
| `-wan-gw` | авто из default-маршрута | next-hop для антилуп-маршрута (**IP шлюза**, не интерфейс) |
| `-wan-iface-list` | `WAN` | interface-list для masquerade |
| `-root-dir` / `-tmpdir` | `usb1/images/awg-converter` / `usb1/tmp` | каталоги на внешнем носителе для `remote-image` |
| `-addr-list` | `to_vpn_list` | address-list выборочной маршрутизации |
| `-rt-table` | `to_vpn_table` | routing table выборочной маршрутизации |
| `-conn-mark` | `to_vpn_mark` | connection-mark policy routing |
| `-wg-name` / `-wg-port` | `awg` / `13231` | имя и порт локального WireGuard-интерфейса |
| `-env-list` | `awg-env` | имя env-списка `/container/envs` |
| `-conf-mount` | `false` | устаревший режим: читать `awg0.conf` из смонтированного файла вместо env-переменных |

**Образ.** Готовый образ публикуется в Docker Hub как **`nskforward/mikwg`**:
`:latest` — последний релиз, `:X.Y.Z` — зафиксированная версия (список тегов —
[hub.docker.com/r/nskforward/mikwg/tags](https://hub.docker.com/r/nskforward/mikwg/tags)).
Роутер забирает его напрямую при установке (`/container/add remote-image=...`),
загружать tar в Files не нужно. Образ содержит только статический бинарник
конвертера. Слой для реестра **сжат gzip** — `remote-image` распаковывает слой
при загрузке (RouterOS ждёт `<digest>.tar.gzip`). Оффлайн-tar, наоборот,
собирается с **несжатым** слоем: импортёр `file=` сжатый не принимает.

**Оффлайн-установка** (роутер без доступа в интернет): соберите tar локально —
без Docker-демона, тем же in-process сборщиком, что формирует публикуемый образ:
```bash
./build-nodocker.sh             # → awg-converter-arm64.tar
```
`./build.sh` — совместимая обёртка над тем же сборщиком; `make image` собирает
Docker-образ для локальной проверки. Сгенерируйте скрипт с пустым `-image`
(`-image ""`), загрузите `awg-converter-arm64.tar` в Files и импортируйте скрипт.

## Обновление

### Параметры обфускации (S/H/HPK/Jc/I1)

Параметры живут в `/container/envs` (список `awg-env`), поэтому пересборка образа
и заливка файлов не нужны. Меняйте по одному значению прямо в RouterOS:

```rsc
# посмотреть текущие значения
/container/envs/print where list=awg-env
# изменить одно значение ...
/container/envs/set [find where list=awg-env && key="S4"] value=16
# ... и применить
/container/restart awg-converter
```

В Winbox: раздел **Container → Envs**, двойной клик по значению, правка, затем
Restart контейнера. После перезапуска проверьте лог
(`/log/print where message~"awg-converter"`): конвертер печатает эффективные
S/H/HPK/Jc/I1 без секретов.

Если провайдер прислал новый `awg0.conf` — сгенерируйте скрипт заново и
импортируйте его: список `awg-env` пересобирается целиком из конфига (ручные
правки envs при этом перезаписываются значениями из файла).

> ⚠️ `HeaderProtectionKey` теперь хранится в env и попадает в `/container/envs/print`
> и `/export` роутера. Это ключ обфускации, а не аутентификации, и он и так
> передаётся контейнеру; приватный ключ WireGuard по-прежнему остаётся только в
> `/interface/wireguard`. Экспорты роутера не публикуйте.

### Образ контейнера

Образ тянется из Docker Hub, поэтому для обновления не нужен ни Winbox, ни tar.

**Одной командой (рекомендуется).** Генерируемый скрипт ставит на роутер
служебный скрипт `awg-update`, который перекачивает образ и перезапускает
контейнер на свежем бинарнике:

```rsc
/system/script/run awg-update
```

`awg-update` сверяет тег контейнера с тегом, под который был сгенерирован
скрипт, при необходимости переключает `remote-image`, выполняет
`/container/update`, перезапускает контейнер и на время загрузки образа ставит
watchdog на паузу. Результат — в логе (`/log/print where message~"awg-converter"`),
строка `mikwg awg-converter <версия>`. Чтобы получать свежие релизы, генерируйте
скрипт с дефолтным образом `nskforward/mikwg:latest` (флаг `-image` можно не
указывать); при пине на конкретную версию `awg-update` перекачивает именно её.
Если скрипт сгенерирован старым `rscgen` (без `awg-update`), обновляйтесь вручную
или перегенерируйте `.rsc` и импортируйте его заново.

**Вручную:**

1. Переключите `remote-image` на новый тег (номер — со
   [страницы релизов](https://github.com/nskforward/mikwg/releases/latest)) и
   обновите образ:
   ```rsc
   /container/set awg-converter remote-image=nskforward/mikwg:X.Y.Z
   /container/update awg-converter
   ```
   `/container/update` перекачивает и распаковывает образ, заменяя старый. Если
   `update` не сработал — `/container/remove awg-converter` и повторный
   `/import file=routeros.generated.rsc`, сгенерированный с новым `-image`.
2. Скрипт, сгенерированный `rscgen` версии ≥ 1.1.2, при импорте сам сначала
   обновляет образ, а затем переключает контейнер на env-режим (снимая старый
   mount), поэтому переход с версий ≤ 1.0.1 делается одним импортом.

> Оффлайн-установка обновляется через `-tar`: загрузите новый
> `awg-converter-arm64.tar` в Files, затем `/container/remove awg-converter` и
> повторный импорт скрипта (служебный `awg-update` в этом режиме не создаётся).

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

- `mangle` в `prerouting` выстроен так, чтобы дорогая проверка выполнялась один
  раз на соединение, а горячее направление не платило за неё вовсе (порядок
  правил важен): (1) трафик **из** туннеля (`in-interface=awg`) сразу
  `accept` и уходит в LAN через main-таблицу — это самое нагруженное,
  «загрузочное» направление; (2) anti-loop; (3) **только для новых соединений**
  (`connection-state=new`) сверяется `to_vpn_list` и ставится
  `mark-connection to_vpn_mark`; (4) по `connection-mark=to_vpn_mark` всем
  пакетам ставится routing-mark `to_vpn_table` (в этой таблице `0.0.0.0/0`
  указывает на `awg`). Установленные пакеты идут по одной дешёвой метке, без
  повторного просмотра списка адресов;
- `mangle` в `output` делает то же для трафика самого роутера (например,
  DNS-запросов) — тоже `connection-state=new` + метка соединения, плюс
  собственный anti-loop;
- трафик к IP AmneziaWG-сервера явно исключён из маркировки (anti-loop в
  `prerouting` и в `output`);
- правило `vpn return` (шаг 1 выше) обязательно и стоит **первым**: иначе ответы
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

Основной источник параметров — **environment-переменные контейнера** (RouterOS
`/container/envs`, список `awg-env`), значения в том же синтаксисе, что и поля
`awg0.conf`:

| Переменная | Пример | Назначение |
|---|---|---|
| `UPSTREAM` | `203.0.113.10:51822` | адрес AmneziaWG-сервера (обязателен) |
| `S1`–`S4` | `129` / `106` / `23` / `12` | префикс-паддинг |
| `H1`–`H4` | `1` / `2` / `3` / `4` (`lo-hi`) | идентификаторы типов |
| `HPK` | base64 | HeaderProtectionKey (псевдоним `HEADERPROTECTIONKEY`) |
| `JC` / `JMIN` / `JMAX` | `3` / `56` / `188` | мусорные пакеты |
| `I1`–`I5` | `<r 226>` | CPS-приманки (пустые не отправляются) |
| `LISTEN` | `0.0.0.0:51820` | адрес, куда шлёт kernel WireGuard (endpoint пира) |
| `JITTER` | `1` | тайминговый джиттер initiation-пакетов |
| `VERBOSE` | `1` | подробный лог |

Имена переменных регистронезависимы, неизвестные игнорируются. Приоритет:
**флаги > env > файл > дефолты**. Если указан `-conf` (или существует
`/etc/awg/awg0.conf`), файл читается как база, а env-переменные перекрывают
отдельные поля; без файла конвертер стартует только на env. `PrivateKey` не
используется и никогда не попадает в контейнер.

Флаги (имеют приоритет над env):

| Флаг | По умолчанию | Описание |
|---|---|---|
| `-conf` | `/etc/awg/awg0.conf` | путь к конфигу; отсутствие файла — не ошибка (env-only) |
| `-listen` | `0.0.0.0:51820` | адрес, куда шлёт kernel WireGuard (env `LISTEN`) |
| `-upstream` | из `UPSTREAM`/`Endpoint` | адрес AmneziaWG-сервера |
| `-jitter` | `false` | тайминговый джиттер (env `JITTER`) |
| `-v` | `false` | подробный лог (env `VERBOSE`) |
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
make push     # сборка и публикация :$(VERSION) в docker.io/nskforward/mikwg
make push-release   # то же + тег latest
```

Публикация образа автоматизирована: пуш тега `vX.Y.Z` запускает GitHub Actions
(`.github/workflows/release.yml`), который прогоняет тесты, пушит
`nskforward/mikwg:X.Y.Z` и `:latest` и прикладывает `awg-converter-arm64.tar` к
GitHub Release. Нужны секреты репозитория `DOCKER_USERNAME`/`DOCKER_PASSWORD`
(пароль — Docker Hub access token с правом push).
Локально `make push` использует `DOCKER_USERNAME`/`DOCKER_PASSWORD` или
`docker login`.

Пакеты:
- `internal/awg` — трансформации проводного формата (ядро решения);
- `internal/config` — парсер `awg0.conf` и environment-переменных;
- `internal/proxy` — UDP-прокси;
- `cmd/awg-converter`, `cmd/rscgen`, `cmd/imagetool`.

Спецификация формата с ссылками на исходники amneziawg-go —
[`docs/protocol-notes.md`](docs/protocol-notes.md). Безопасные операции на
RouterOS — [`docs/safe-operations.md`](docs/safe-operations.md).

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

Остаётся ровно одна аллокация на пакет — `chacha20.Cipher` (384 B). Её снимает
отложенный этап F (собственная block-функция ChaCha20 с предвычисленным ключом) —
см. TODO в [`PLAN.md`](PLAN.md). Фактические цифры на роутере (пропускная
способность, загрузка CPU) — в разделе приёмки плана.

## Устранение неполадок

- **`remote-image`: `download/extract error: extract layer failed`.** Слой образа
  не сжат. `remote-image` всегда распаковывает слой при загрузке, поэтому образ
  в реестре обязан иметь **gzip**-слой (официальный `nskforward/mikwg` — имеет).
  Оффлайн-tar, наоборот, требует **несжатый** слой.
- **Контейнер стартует, но `last-handshake` не обновляется.** Проверьте лог
  контейнера (источник конфига и распознанные S/H/HPK), значения
  `/container/envs/print where list=awg-env` (`UPSTREAM` должен указывать на
  сервер, `S1–S4 ≥ 12` при заданном `HPK`), доступность адреса сервера и что
  антилуп-маршрут до IP сервера указывает на реальный WAN next-hop, а не на
  интерфейс.
- **Handshake проходит, data не идёт.** Почти всегда — нет srcnat в туннель
  (`out-interface=awg`) либо reply-loop из-за порядка mangle-правил; разбор в
  разделе «Выборочная маршрутизация».
- **После перезагрузки контейнер не поднялся.** Проверьте `start-on-boot=yes` и
  что внешний носитель смонтирован (образ лежит на нём).
- **ICMP через туннель не проходит.** На многих VPN-серверах это ожидаемо; TCP
  при этом работает. Не считайте это поломкой.

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
