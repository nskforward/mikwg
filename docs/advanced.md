# Продвинутая установка и обновление

Дополнительные сведения к разделу «Быстрый старт» в
[`README.md`](../README.md): флаги генератора `rscgen`, детали образа
контейнера, оффлайн-установка и ручное обновление.

## Флаги `rscgen`

Все флаги необязательные; значения по умолчанию подходят для большинства.

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

> ⚠️ Антилуп-маршрут до IP сервера должен указывать на **реальный next-hop**
> (`gateway=192.168.1.1`), а не на интерфейс (`gateway=ether1`). Interface-route
> заставляет RouterOS ARP-ить публичный IP прямо в LAN-сегменте WAN, что на
> обычном ethernet-аплинке не срабатывает и полностью ломает туннель.
> `rscgen` определяет next-hop автоматически; переопределить можно флагом
> `-wan-gw` (именно IP шлюза, не интерфейс).

## Образ контейнера

Готовый образ публикуется в Docker Hub как **`nskforward/mikwg`**:
`:latest` — последний релиз, `:X.Y.Z` — зафиксированная версия (список тегов —
[hub.docker.com/r/nskforward/mikwg/tags](https://hub.docker.com/r/nskforward/mikwg/tags)).
Роутер забирает его напрямую при установке (`/container/add remote-image=...`),
загружать tar в Files не нужно. Образ содержит только статический бинарник
конвертера.

Слой для реестра **сжат gzip** — `remote-image` распаковывает слой при загрузке
(RouterOS ждёт `<digest>.tar.gzip`). Оффлайн-tar, наоборот, собирается с
**несжатым** слоем: импортёр `file=` сжатый не принимает.

## Оффлайн-установка

Для роутера без доступа в интернет соберите tar локально — без Docker-демона,
тем же in-process сборщиком, что формирует публикуемый образ:

```bash
./build-nodocker.sh             # → awg-converter-arm64.tar
```

`./build.sh` — совместимая обёртка над тем же сборщиком; `make image` собирает
Docker-образ для локальной проверки. Сгенерируйте скрипт с пустым `-image`
(`-image ""`), загрузите `awg-converter-arm64.tar` в Files и импортируйте скрипт.

> Оффлайн-установка обновляется через `-tar`: загрузите новый
> `awg-converter-arm64.tar` в Files, затем `/container/remove awg-converter` и
> повторный импорт скрипта (служебный `awg-update` в этом режиме не создаётся).

## Ручное обновление образа

Если служебный скрипт `awg-update` недоступен (например, `.rsc` сгенерирован
старым `rscgen`), обновляйтесь вручную:

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

## Параметры обфускации по отдельности

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

> ⚠️ `HeaderProtectionKey` хранится в env и попадает в `/container/envs/print`
> и `/export` роутера. Это ключ обфускации, а не аутентификации, и он и так
> передаётся контейнеру; приватный ключ WireGuard по-прежнему остаётся только в
> `/interface/wireguard`. Экспорты роутера не публикуйте.
