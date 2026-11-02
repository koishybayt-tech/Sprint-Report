# Как поднять сервис

## 0. Что нужно заранее
| Что | Где взять | Кто |
|---|---|---|
| Сервер Linux (Ubuntu 22.04/24.04), 2 vCPU, 4 ГБ RAM, 40 ГБ SSD | любой VPS/облако или внутренний сервер | DevOps |
| **Доступ сервера к jira.adata.kz** | если Jira только из офиса/VPN — сервер внутри сети или с туннелем | DevOps |
| Поддомен, например `reports.adata.kz`, A-запись на IP сервера | DNS | DevOps |
| Jira PAT сервисного пользователя (только чтение) | Jira → Профиль → Personal Access Tokens | админ Jira |
| Ключ API отпусков Bitrix (`BITRIX_API_KEY`) | владелец bitrix.avh.kz | админ Bitrix |
| Telegram-бот | @BotFather → /newbot; затем /setprivacy → **Disable** | вы |
| Бот — администратор группы «Adata Group» | настройки группы | админ группы |
| Экспорт истории группы (JSON) | Telegram Desktop → группа → ⋮ → Export chat history → JSON, без медиа | админ группы |

## 1. Подготовка сервера
```bash
sudo apt update && sudo apt install -y git ca-certificates curl ufw age
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker $USER && newgrp docker
sudo ufw allow OpenSSH && sudo ufw allow 80,443/tcp && sudo ufw enable
```

## 2. Код
```bash
sudo mkdir -p /opt/adata-reports && sudo chown $USER /opt/adata-reports
git clone git@github.com:<org>/adata-reports.git /opt/adata-reports   # репозиторий ПРИВАТНЫЙ
cd /opt/adata-reports
```

## 3. Настройки
```bash
cp deploy/.env.example deploy/.env && chmod 600 deploy/.env
openssl rand -hex 32   # → SESSION_SECRET
openssl rand -hex 24   # → TG_WEBHOOK_SECRET
nano deploy/.env       # DOMAIN, POSTGRES_PASSWORD, JIRA_PAT, BITRIX_API_KEY, TG_*, ADMIN_TG_IDS
```
Проверка Jira-токена с сервера:
```bash
curl -s -o /dev/null -w '%{http_code}\n' -H "Authorization: Bearer $(grep ^JIRA_PAT deploy/.env | cut -d= -f2)" https://jira.adata.kz/rest/api/2/myself   # ждём 200
```

## 4. Запуск
```bash
make up                                    # сборка и старт всех контейнеров
docker compose -f deploy/docker-compose.yml ps
docker compose -f deploy/docker-compose.yml exec api /app/reports migrate
```
Caddy сам получит HTTPS-сертификат, когда DNS указывает на сервер.

## 5. Начальные данные
```bash
# первый администратор; пароль печатается один раз (или задайте его в REPORTS_NEW_PASSWORD)
$C exec api /app/reports users add --login <логин> --name "<ФИО>" --role admin --employee <логин Jira>
C="docker compose -f deploy/docker-compose.yml"
$C cp seed api:/tmp/seed && $C exec api /app/reports seed --dir /tmp/seed      # сотрудники, алиасы TG, праздники
$C exec api /app/reports sync jira --full                                      # 5–15 минут
$C exec api /app/reports sync bitrix
$C cp ~/result.json api:/tmp/result.json && $C exec api /app/reports import-telegram --file /tmp/result.json   # история; повторный импорт безопасен
# или HTML-экспорт (формат по умолчанию): скопируйте messages*.html в папку и укажите её: --file /tmp/tgexport
# бот: worker сам принимает сообщения (getUpdates), если в .env заданы TG_BOT_TOKEN и TG_CHAT_ID
```

## 6. Проверка
- `https://reports.adata.kz` открывается, вход через Telegram работает для ID из `ADMIN_TG_IDS`.
- «Спринты» совпадают с текущим дашбордом (сравнить 3–4 проекта по числам).
- Написать в группу тестовое «Пропуск 9:00-9:10» → через пару секунд событие видно в «Посещаемости»; удалить через UI.
- `/admin/review` — очередь не пустая только для действительно странных сообщений.
- Админка → «Синхронизации»: последний запуск Jira зелёный, worker пишет его каждые 15 мин.

## 7. Бэкапы
```bash
age-keygen -o ~/backup.key   # публичный ключ → BACKUP_AGE_RECIPIENT, сам ключ хранить ВНЕ сервера
(crontab -l; echo "30 2 * * * /opt/adata-reports/deploy/backup.sh >> /var/log/adata-backup.log 2>&1") | crontab -
```
Восстановление: `age -d -i backup.key f.sql.gz.age | gunzip | docker compose -f deploy/docker-compose.yml exec -T postgres psql -U reports reports`.

## Обновление
```bash
cd /opt/adata-reports && make deploy
```
Откат: `git checkout <прошлый тег> && make up`.

## Локальная разработка
```bash
# весь стек в docker без HTTPS: api и web — только на этом компьютере (:8080, :3000),
# коллегам в офисной сети — http://<IP компьютера>/, вход — пользователями сервиса («Настройки» → «Пользователи»)
# (порты меняются через API_PORT / WEB_PORT / LAN_PORT)
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.local.yml --env-file deploy/.env up -d --build
make dev        # postgres в docker, api :8080, web :3000 (прокси /api → 8080)
make test
make tgparse    # метрики парсера на корпусе
```
Для бота локально — `ngrok http 8080` и `reports tg set-webhook --url https://<ngrok>/api/v1/tg/webhook` (или long polling: `reports tg poll`).

## Частые проблемы
| Симптом | Причина |
|---|---|
| Бот молчит, сообщений нет | privacy mode не выключен, или бот не админ, или webhook не установлен (`reports tg webhook-info`) |
| Jira 401 | PAT истёк или пользователь без доступа к проекту |
| Jira таймаут | сервер не видит jira.adata.kz (сеть/VPN) |
| Сертификат не выдаётся | DNS ещё не обновился или закрыт 80 порт |
| Посещаемость у кого-то пустая | автор не сопоставлен — смотреть «Кто это?» в `/admin/review` |
