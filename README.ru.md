# fb-agent

[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go&logoColor=white)](https://go.dev/)
[![Лицензия: MIT](https://img.shields.io/badge/Лицензия-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Linux](https://img.shields.io/badge/ОС-Linux-FCC624?style=flat&logo=linux&logoColor=black)](https://kernel.org)
[![Статический бинарник](https://img.shields.io/badge/Бинарник-Статический_(CGO__off)-success)](https://github.com/razqqm/fb-agent)
[![Fluent Bit](https://img.shields.io/badge/Fluent_Bit-3.x-49BDA5?style=flat&logo=fluentbit&logoColor=white)](https://fluentbit.io/)
[![VictoriaLogs](https://img.shields.io/badge/VictoriaLogs-совместим-6C2DC7?style=flat)](https://docs.victoriametrics.com/victorialogs/)
[![Арх: amd64](https://img.shields.io/badge/арх-amd64-blue)](https://github.com/razqqm/fb-agent)
[![Арх: arm64](https://img.shields.io/badge/арх-arm64-blue)](https://github.com/razqqm/fb-agent)

Однофайловый инфраструктурный агент для управления жизненным циклом [Fluent Bit](https://fluentbit.io/). Устанавливает, настраивает, мониторит и регистрирует хосты — всё в одном статическом Go-бинарнике.

**[English documentation →](README.md)**

---

## Возможности

- **Один бинарник** — без зависимостей, без интерпретаторов, один файл для деплоя
- **Автодетекция** — ОС, окружение (LXC/Docker/VM/bare-metal) и сервисы с версиями
- **Генерация конфигов** — конфиги Fluent Bit из шаблонов с обнаруженными сервисами
- **Обнаружение сервисов** — SSH, Nginx, PostgreSQL, Redis, Kerio Connect, Rocket.Chat, Fail2Ban, Docker, HAProxy и 15+ других
- **Регистрация хостов** — собирает fingerprint (IP, порты, железо, сервисы) и отправляет в [VictoriaLogs](https://docs.victoriametrics.com/victorialogs/)
- **mTLS** — автоматическая генерация и подпись сертификатов через Go crypto
- **Режим демона** — заменяет 4 отдельных systemd таймера одним сервисом (watchdog + регистрация + обновление сертификатов)
- **Мониторинг связи** — state machine с алертом при потере связи (по умолчанию: 6 часов)
- **Кросс-платформа** — сборка для `linux/amd64` и `linux/arm64`

## Быстрый старт

```bash
# Сборка
make build

# Или вручную
CGO_ENABLED=0 go build -o fb-agent .

# Установить Fluent Bit на хост (нужен root)
sudo ./fb-agent install

# Проверить статус
./fb-agent status

# Зарегистрировать хост в VictoriaLogs
sudo ./fb-agent register

# Запустить как демон (замена cron/таймеров)
sudo ./fb-agent daemon
```

## Команды

| Команда | Описание |
|---------|----------|
| `install` | Установить Fluent Bit, обнаружить сервисы, сгенерировать конфиг, запустить |
| `register` | Собрать fingerprint хоста, отправить в VictoriaLogs |
| `watchdog` | Разовая проверка связи и здоровья |
| `daemon` | Долгоживущий режим: watchdog + register + обновление сертификатов |
| `uninstall` | Остановить и удалить Fluent Bit (`--purge` для полной очистки) |
| `status` | Показать здоровье агента, связь, сертификаты |
| `version` | Версия и информация о сборке |

## Конфигурация

Вся конфигурация через переменные окружения — никаких конфиг-файлов для самого агента.

| Переменная | По умолчанию | Описание |
|------------|-------------|----------|
| `VL_HOST` | `localhost` | Хост VictoriaLogs |
| `VL_PORT` | `443` | Порт VL (443=HTTPS, 9428=HTTP, 9429=mTLS) |
| `FB_HOSTNAME` | hostname ОС | Переопределить hostname |
| `FB_JOB` | автоопределение | Метка окружения: `lxc`, `remote`, `docker`, `vm` |
| `FB_LOG_PATHS` | — | Доп. лог-файлы (через двоеточие) |
| `FB_EXTRA_TAGS` | — | Теги для доп. лог-файлов (через двоеточие) |
| `FB_BUFFER_SIZE` | авто по RAM | Размер файлового буфера |
| `FB_GZIP` | авто | Сжатие: `on`/`off` (авто: on для remote) |
| `FB_FLUSH` | `5` | Интервал flush в секундах |
| `FB_SKIP_DETECT` | — | `1` = пропустить автодетекцию сервисов |
| `FB_SKIP_MTLS` | — | `1` = пропустить mTLS |
| `CF_CLIENT_ID` | — | Cloudflare Access service token ID |
| `CF_CLIENT_SECRET` | — | Cloudflare Access service token secret |

## Как это работает

### Процесс установки

1. Определяет ОС (Debian, Ubuntu, Alpine, RHEL и др.)
2. Добавляет репозиторий Fluent Bit (с фолбэками: trixie→bookworm, oracular→noble)
3. Устанавливает Fluent Bit через пакетный менеджер
4. Определяет окружение (LXC, Docker, VM, bare-metal)
5. Обнаруживает запущенные сервисы и пути к их логам
6. Генерирует `fluent-bit.conf` из встроенных шаблонов
7. Деплоит встроенные `enrich.lua` и кастомные парсеры
8. Опционально регистрирует mTLS сертификаты
9. Настраивает systemd с hardened unit (LimitNOFILE, ProtectSystem, OOMScoreAdjust)
10. Запускает Fluent Bit и демон fb-agent

### Режим демона

Заменяет отдельные systemd таймеры одним сервисом:

- **Каждые 5 мин** — watchdog: проверка health endpoint + output retries
- **Каждые 24ч** — register: обновление fingerprint в VictoriaLogs
- **Каждые 7 дней** — проверка сертификатов (перевыпуск если <30 дней)
- **Алерт** — если offline >6 часов → alert file + syslog

### Данные регистрации

Команда `register` собирает и отправляет:

```json
{
  "host_id": "fingerprint на основе machine-id",
  "hostname": "myhost",
  "internal_ip": "10.0.1.3",
  "external_ip": "203.0.113.1",
  "os": "Debian GNU/Linux 13 (trixie)",
  "environment": "lxc",
  "cpu": "4x AMD EPYC",
  "ram_mb": 4096,
  "open_ports": "22,80,443,5432",
  "services": [{"Name": "SSH", "Status": "active", "Version": "OpenSSH_10.0"}]
}
```

## Сборка

```bash
# Обе архитектуры
make build

# Одна архитектура
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o fb-agent .

# С информацией о версии
go build -ldflags "-s -w -X main.version=1.0.0 -X main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o fb-agent .
```

## Верификация

```bash
python3 verify.py
```

52 автоматических проверки: сборка, линтинг (golangci-lint), орфография, качество кода, соответствие спецификации, паритет с bash, тест бинарника.

## Требования

- **Сборка**: Go 1.21+
- **Runtime**: Linux (systemd), root для install/register/daemon
- **Цель**: Fluent Bit 3.x, VictoriaLogs

## Лицензия

MIT
