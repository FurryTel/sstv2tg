# sstv2tg

A SIP extension that receives slow-scan television. Call it, wait for the
beep, play an SSTV transmission into the phone, and the decoded picture is
posted to a Telegram chat.

It registers with a SIP server (FreeSWITCH, Kamailio, or anything else that
takes a plain UDP registration), answers with G.711 PCMU or PCMA, detects
the SSTV mode from the VIS header, follows each line's sync pulse through
lost packets and clock drift, and posts the image with the Telegram Bot API.
A transmission cut short posts the lines that arrived.

## Supported modes

- Robot 36, 72, 24; Robot 8, 12, 24 B/W
- Martin M1, M2
- Scottie S1, S2, DX
- Wraase SC2-120, SC2-180
- PD 50, 90, 120, 160, 180, 240, 290
- Pasokon P3, P5, P7

## Configuration

Set by environment variables.

| Var | Purpose | Default |
|---|---|---|
| `SSTV_SIP_SERVER` | SIP server host[:port] | required |
| `SSTV_SIP_USER` | Registration user | required |
| `SSTV_SIP_PASS` | Registration password | required |
| `SSTV_SIP_LOCAL_PORT` | UDP bind port | `5060` |
| `SSTV_MAX_CALLS` | Concurrent call cap | `4` |
| `SSTV_RTP_PORT_RANGE` | RTP port range, `low-high` | `16384-16484` |
| `SSTV_ADVERTISED_IP` | IP written into SDP `c=`; empty means detect | |
| `SSTV_TG_BOT_TOKEN` | Telegram bot token | required |
| `SSTV_TG_CHAT_ID` | Destination chat ID | required |
| `SSTV_TG_THREAD_ID` | Forum topic ID; empty or `0` means the main chat | |
| `SSTV_TG_CAPTION` | Caption on every post; empty means none | |
| `SSTV_VIS_TIMEOUT` | How long to wait for a VIS header after the beep | `60s` |
| `SSTV_SILENCE_TIMEOUT` | Silence that ends a transmission partway | `2s` |
| `SSTV_LOG_LEVEL` | `debug`, `info`, `warn` or `error` | `info` |

## Running

### Docker

The SDP has to advertise an address the SIP server can reach, so run with
host networking on Linux.

```bash
docker build -t sstv2tg .
docker run -d --restart unless-stopped --name sstv2tg --network=host --env-file .env sstv2tg
```

### Debian package

Tagged releases publish `amd64` and `arm64` debs. To build one locally,
run `goreleaser release --snapshot --clean` and take it from `dist/`.

```bash
sudo dpkg -i sstv2tg_*_linux_amd64.deb
sudoedit /etc/sstv2tg/env
sudo systemctl enable --now sstv2tg
```

Upgrades keep `/etc/sstv2tg/env` and restart the service.

## Sending a picture

Any SSTV encoder app works. Hold the phone's speaker to the handset
mouthpiece, not a speakerphone: a speakerphone's noise suppression and echo
mangle the tones. Softphones and phones with noise suppression need it
turned off.

## Releasing

Push a `v*` tag. The release workflow runs the tests and GoReleaser, which
attaches the debs to a GitHub release.

## Development

```bash
go test ./...
```

The call-flow tests place real calls over loopback SIP and take about 40
seconds.
