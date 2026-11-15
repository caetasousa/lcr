#!/usr/bin/env bash
# Instala/atualiza o LCR Assina no ubuntu-server, no mesmo padrão dos outros apps:
# usuário de sistema dedicado + Docker rootless + linger, porta só no loopback.
# Uso: sudo bash deploy/install-ubuntu-server.sh   (rodar a partir da cópia do projeto)
set -euo pipefail

APP_USER=lcr
PORTA=8482
PUBLIC_URL=https://lcr.d4ffy.com
SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEST=/home/$APP_USER/app

[[ $EUID -eq 0 ]] || { echo "Rode com sudo." >&2; exit 1; }

echo "==> Dependências (docker-buildx para o compose build)"
if ! dpkg -s docker-buildx >/dev/null 2>&1; then
  # Espera a trava do apt (apt-daily roda logo após o boot do WSL).
  APT="apt-get -o DPkg::Lock::Timeout=300"
  $APT update -qq && $APT install -y -qq docker-buildx \
    || echo "AVISO: docker-buildx não instalou; uso o builder clássico do Docker." >&2
fi

echo "==> Usuário $APP_USER"
id "$APP_USER" >/dev/null 2>&1 || useradd -m -s /bin/bash "$APP_USER"
UID_APP=$(id -u "$APP_USER")
if ! grep -q "^$APP_USER:" /etc/subuid; then
  ULTIMO=$(awk -F: '{e=$2+$3; if (e>m) m=e} END {print m+0}' /etc/subuid /etc/subgid)
  [[ $ULTIMO -lt 100000 ]] && ULTIMO=100000
  usermod --add-subuids "$ULTIMO-$((ULTIMO+65535))" --add-subgids "$ULTIMO-$((ULTIMO+65535))" "$APP_USER"
fi
loginctl enable-linger "$APP_USER"
systemctl start "user@$UID_APP.service"
for _ in $(seq 20); do [[ -S /run/user/$UID_APP/bus ]] && break; sleep 0.5; done

como_app() {
  runuser -u "$APP_USER" -- env HOME=/home/$APP_USER \
    XDG_RUNTIME_DIR=/run/user/$UID_APP \
    DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/$UID_APP/bus \
    DOCKER_HOST=unix:///run/user/$UID_APP/docker.sock \
    PATH=/usr/share/docker.io/contrib:/usr/local/bin:/usr/bin:/bin \
    bash -c "$1"
}

echo "==> Docker rootless para $APP_USER"
if [[ ! -f /home/$APP_USER/.config/systemd/user/docker.service ]]; then
  como_app 'dockerd-rootless-setuptool.sh install'
fi
como_app 'systemctl --user enable --now docker'
grep -q DOCKER_HOST /home/$APP_USER/.bashrc 2>/dev/null || {
  echo "export XDG_RUNTIME_DIR=/run/user/$UID_APP"
  echo "export DOCKER_HOST=unix:///run/user/$UID_APP/docker.sock"
} >> /home/$APP_USER/.bashrc
for _ in $(seq 30); do como_app 'docker info >/dev/null 2>&1' && break; sleep 1; done

echo "==> Código em $DEST"
mkdir -p "$DEST"
tar -C "$SRC" --exclude=node_modules --exclude=frontend/dist --exclude=.env --exclude=.git -cf - . | tar -C "$DEST" -xf -

if [[ ! -f $DEST/.env ]]; then
  echo "==> Gerando .env com segredos aleatórios"
  gera() { tr -dc 'A-Za-z0-9' </dev/urandom | head -c "$1"; }
  cat > "$DEST/.env" <<ENV
POSTGRES_PASSWORD=$(gera 32)
AC_SENHA=$(gera 40)
PUBLIC_URL=$PUBLIC_URL
ENV
else
  echo "==> .env já existe: mantido (AC_SENHA não pode mudar, senão as chaves da AC ficam ilegíveis)"
fi
chmod 600 "$DEST/.env"
# Só a pasta do app e o .bashrc. NUNCA "chown -R /home/lcr": lá dentro fica o
# armazenamento do Docker rootless (volumes e camadas de imagem), cujos arquivos
# pertencem a subuids (ex.: postgres = uid 70 no container). Mudar o dono deles
# deixa o PostgreSQL sem permissão nos próprios arquivos.
chown -R "$APP_USER:$APP_USER" "$DEST"
chown "$APP_USER:$APP_USER" /home/$APP_USER/.bashrc

echo "==> Build e subida (pode levar alguns minutos)"
COMPOSE="docker compose -p lcr -f docker-compose.yml -f deploy/docker-compose.prod.yml"
if dpkg -s docker-buildx >/dev/null 2>&1; then
  como_app "cd $DEST && $COMPOSE build && $COMPOSE up -d"
else
  como_app "cd $DEST && DOCKER_BUILDKIT=0 docker build -t lcr-app:latest . && $COMPOSE up -d --no-build"
fi

echo "==> Aguardando a aplicação em 127.0.0.1:$PORTA"
for _ in $(seq 60); do
  if curl -fsS "http://127.0.0.1:$PORTA/api/ac" >/dev/null 2>&1; then
    echo "OK: no ar em $PUBLIC_URL"
    exit 0
  fi
  sleep 2
done
echo "A aplicação não respondeu. Veja os logs:" >&2
como_app "cd $DEST && docker compose -p lcr logs --tail=50 app" >&2
exit 1
