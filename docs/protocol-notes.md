# AmneziaWG 3.x — спецификация проводного формата (для конвертера)

Источник: исходники `github.com/amnezia-vpn/amneziawg-go` (`v3`), файлы
`device/send.go`, `device/receive.go`, `device/noise-protocol.go`,
`device/noise-types.go`, `device/cookie.go`, `device/obf*.go`, `device/uapi.go`.
Все ссылки на строки — на клон коммита, использованный при разработке.

## 1. Константы

| Константа | Значение |
|---|---|
| `MessageInitiationSize` | 148 |
| `MessageResponseSize` | 92 |
| `MessageCookieReplySize` | 64 |
| `MessageTransportHeaderSize` | 16 |
| `MessageTransportSize` | 32 (16 header + 16 Poly1305 tag) |
| Типы сообщений | Unknown=0, Initiation=1, Response=2, CookieReply=3, Transport=4 |
| `HeaderCipherKeySize` / `HeaderCipherNonceSize` | 32 / 12 |

## 2. Шифрование заголовка (HeaderProtection)

`HeaderProtectionCipher(salt) = chacha20.NewUnauthenticatedCipher(key, salt)`
— стандартный **ChaCha20 IETF** (`golang.org/x/crypto/chacha20`), 32-байтовый
ключ = `HeaderProtectionKey`, 12-байтовый nonce = **первые 12 байт датаграммы**
(то есть начало S-паддинга), счётчик с 0.

Что именно XOR-ится кейстримом (позиции с 0):

| Тип | Домен HP |
|---|---|
| Initiation | всё каноническое сообщение (148 Б) |
| Response | всё каноническое сообщение (92 Б) |
| CookieReply | всё каноническое сообщение (64 Б) |
| Transport | только первые 16 байт (type+receiver+counter), шифротекст не трогается |

Требование валидности: при заданном `HeaderProtectionKey` все `S1..S4 ≥ 12`
(проверка в `uapi.go`). В рабочем конфиге 129/106/23/12 — выполняется.
Если ключ нулевой — HP не применяется.

## 3. Проводной формат

Обозначения: `pad(S)` — S случайных байт; `HP(x)` — XOR кейстримом ChaCha20 из §2;
`msg` — каноническое сообщение WireGuard.

```
Initiation (client→server):
    pad(S1) || HP(msg_148)                       (+ случайный трейлер, если random_trailers)

Response (server→client):
    pad(S2) || HP(msg_92)                        (+ трейлер?)

CookieReply (server→client):
    pad(S3) || HP(msg_64)                        (+ трейлер?)

Transport (оба направления):
    pad(S4) || HP(msg[0:16]) || msg[16:]         (+ трейлер?)
```

- Байт `msg[0:4]` — little-endian идентификатор `H1..H4`, выбранный случайно из
  своего диапазона (для нашего конфига H=1..4, т.е. совпадает со стандартными
  типами WG). Установлен ДО `HP`.
- `msg[4:16]` транспорта = receiver index (4) + counter (8).
- AEAD транспорта (`Open(content, nonce, content, nil)`, `device/receive.go:297`)
  **не аутентифицирует заголовок** (AAD=nil), поэтому вставка паддинга и замена
  типа прозрачны для криптографии.
- MAC1/MAC2 (`AddMacs`, `device/cookie.go:216`) считаются над **каноническим**
  сообщением ДО `HP` (в `device/send.go` `AddMacs` вызывается перед
  `XORKeyStream`). Поэтому конвертеру пересчитывать MAC не нужно. Криптостойкость
  не затрагивается.
- `ContentPaddingAddition` добавляется к **открытому тексту** (внутреннему
  IP-пакету) до `Seal` (`device/send.go:607-620`), то есть лежит внутри AEAD и
  снаружи не воспроизводима. Получатель получает IP-пакет + нулевой хвост;
  лишнее отсекается по IP total length (проверяется на стенде/роутере).

## 4. Приманки перед initiation

Порядок отправки (`device/send.go:139-147`):
```
[ I1 ] [ I2 ] [ I3 ] [ I4 ] [ I5 ]   — CPS-пакеты (только те, что заданы)
[ junk × Jc ]                         — мусорные пакеты
[ initiation ]                        — (см. §3)
```
Отправляются одним потоком на тот же endpoint.

- **Junk**: `count = Jc`, длина каждого `= Jmin + fastrandn(Jmax-Jmin)`, т.е.
  диапазон `[Jmin, Jmax)` (Jmax исключается); содержимое — `crypto/rand`.
  (`JunkPackets`, `device/noise-protocol.go:632`).
- **CPS (I1–I5)**: одиночные UDP-датаграммы, собранные из obf-цепочки
  (`device/obf.go`). Грамматика тегов:
  `<b HEX>` — литеральные байты; `<t>` — 4 Б BE unix-время; `<r N>` — N случайных
  байт; `<rc N>` — N случайных букв; `<rd N>` — N случайных цифр; `<d>` — данные;
  `<ds>` — данные base64; `<dz N>` — N байт BE длины данных.
  Для нашего конфига `I1 = <r 226>` → 226 случайных байт.
- Сервер приманки игнорирует (его `DeterminePacketTypeAndPadding` их не
  распознаёт и молча дропает).

## 5. Определение типа получателем

`DeterminePacketTypeAndPadding` (`device/receive.go:605`): для каждого типа
проверяет размер `≈ S + MessageXSize` (или `>`, если включены random_trailers) и
сверяет расшифрованное поле типа с диапазоном `H`:

```
typeHash = ChaCha20_keystream[0:4]        // XOR кейстрима в 4 нулевых байта
header = LE(msg_wire[padding : padding+4]) XOR typeHash
if H.Contains(header): это нужный тип
```

Диапазоны `H1..H4` не должны пересекаться (проверка `uapi.go:828`).

## 6. Тайминги (one-sided)

`rekey_after_time`, `rekey_timeout`, `reject_after_time`, `keepalive_timeout`,
`max_handshake_attempts` — диапазоны; влияют только на поведение отправителя и не
проверяются получателем. Дефолты kernel-WG (rekey 120 c ∈ 115–141, retry 5 c ∈ 5–8)
уже попадают в диапазоны рабочего конфига; keepalive выставляется
`persistent-keepalive=12s`. Точная мимикрия таймингов невозможна без
WG-клиента — не требуется для работы.

## 7. Выводы для конвертера

1. HP-ключ и S/H — двусторонние, конвертер берёт их из `awg0.conf`.
2. MAC не пересчитывается: конвертер снимает HP и S, ядро RouterOS проверяет
   канонический WireGuard как обычно.
3. Outbound: из канонического пакета ядра строится AWG-датаграмма; для
   initiation сначала отправляются CPS (I1), затем junk (Jc), затем initiation.
4. Inbound: по смещению S расшифровывается тип, снимается HP и паддинг,
   тип приводится к каноническому (`1/2/3/4 + 00 00 00`), пакет отдаётся ядру.
5. `ContentPaddingAddition` и тайминги — не воспроизводятся; ограничения
   задокументированы. `RandomTrailers` на сервере должен быть выключен
   (в рабочем конфиге отсутствует).
