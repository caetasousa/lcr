# 🏋️ Teste de carga

> 🗣️ **Em palavras simples:** simulamos **60 pessoas fazendo a mesma coisa no mesmo segundo** (criar conta, entrar, assinar, listar, baixar, validar) no `ubuntu-server`. Sem proteção, o servidor **caía em 3,5 segundos** por falta de memória. Com os limitadores, **todas as 360 operações funcionaram**, usando no máximo 782 MiB de RAM.

## 🔥 O problema

As operações com senha usam **Argon2id**, que gasta **64 MiB de RAM de propósito** (é isso que torna ataques de força bruta caros). 60 delas ao mesmo tempo pedem ~3,8 GiB, mais que a VM inteira (4 GB, dividida com outros apps). Os PDFs também são carregados inteiros na memória várias vezes durante assinatura, cifra e validação.

## 🛠️ A solução: filas

| Limitador | Onde | Padrão | Efeito |
|---|---|---|---|
| 🧂 `LIMITE_ARGON2` | [`cripto/cripto.go`](../backend/internal/adapter/secondary/cripto/cripto.go) | 4 | no máximo 4 cálculos Argon2id juntos (4 × 64 MiB); os demais esperam |
| 📄 `LIMITE_PDF` | [`http/router.go`](../backend/internal/adapter/primary/http/router.go) | 4 | no máximo 4 requisições com PDF juntas (assinar, validar, baixar, `.pfx`); depois de 30 s na fila → **503** com `Retry-After` |
| 🧹 `GOMEMLIMIT` | `docker-compose.yml` | 768MiB | o coletor de lixo do Go devolve memória mais cedo |
| 🧱 `mem_limit` | [`deploy/docker-compose.prod.yml`](../deploy/docker-compose.prod.yml) | 1,5 GiB (app) | rede de segurança: se algo escapar, só o LCR reinicia |

Trocar memória por fila: as respostas ficam mais lentas no pico, mas **nada cai**.

## 📊 Resultados no `ubuntu-server`

Instalação de **teste isolada** (banco próprio, porta 8490, container limitado a 1,5 GiB sem swap), 60 usuários simultâneos em cada etapa, mesmo hardware da produção.

### ❌ Antes (sem limitadores), PDF de 500 KB

| Etapa | OK | Erros |
|---|---:|---:|
| cadastro | 0 | 60 |
| todas as seguintes | 0 | 60 cada |

💥 O kernel matou o processo após 3,5 s: `Memory cgroup out of memory: Killed process (lcr) anon-rss:1567232kB`. **0 de 360** operações.

### ✅ Depois (com limitadores), PDF de 500 KB

| Etapa | OK | Erros | p50 | p95 | máx |
|---|---:|---:|---:|---:|---:|
| 🪪 cadastro | 60 | 0 | 11,0 s | 12,3 s | 12,4 s |
| 🔑 login | 60 | 0 | 1,6 s | 3,1 s | 3,2 s |
| 🖋️ assinar | 60 | 0 | 2,2 s | 3,9 s | 4,0 s |
| 📂 listar | 60 | 0 | 48 ms | 59 ms | 60 ms |
| 📥 baixar | 60 | 0 | 1,9 s | 3,5 s | 3,5 s |
| 🔍 validar | 60 | 0 | 200 ms | 374 ms | 383 ms |

🧠 **Pico de memória: 606 MiB.** Nenhum OOM. **360 de 360** operações.

### ✅ Depois, PDF de 5 MB

| Etapa | OK | Erros | p50 | p95 | máx |
|---|---:|---:|---:|---:|---:|
| 🪪 cadastro | 60 | 0 | 7,8 s | 9,1 s | 9,1 s |
| 🔑 login | 60 | 0 | 1,3 s | 2,5 s | 2,6 s |
| 🖋️ assinar | 60 | 0 | 4,7 s | 9,4 s | 9,5 s |
| 📂 listar | 60 | 0 | 15 ms | 20 ms | 22 ms |
| 📥 baixar | 60 | 0 | 2,1 s | 3,8 s | 3,9 s |
| 🔍 validar | 60 | 0 | 905 ms | 1,6 s | 1,8 s |

🧠 **Pico de memória: 782 MiB.** Nenhum OOM. **360 de 360** operações.

### 🧾 Como ler

* ⏳ **Cadastro é o mais lento** porque cada um gera um par RSA-3072 e faz 2 cálculos Argon2id. 60 cadastros **no mesmo segundo** não acontecem na prática. Em uso normal (um por vez), leva ~1 s.
* 🔑 Login, assinar e baixar ficam entre 2 e 4 s no **pior momento** (60 juntos) e voltam a ~0,2 a 0,3 s sem fila.
* 🎛️ Para respostas mais rápidas no pico, aumente `LIMITE_ARGON2` (cada vaga a mais custa 64 MiB de RAM).

## ▶️ Rodar o teste

⚠️ Use **sempre uma instalação de teste**: o gerador cria contas de verdade no banco.

```bash
docker compose up -d --build                      # instalação local de teste
cd backend
go run ./cmd/carga -url http://localhost:8080 -usuarios 60 -kb 500
```

| Opção | Padrão | Significado |
|---|---|---|
| `-url` | `http://localhost:8080` | onde está o LCR Assina |
| `-usuarios` | 60 | quantos usuários fazem cada etapa **ao mesmo tempo** |
| `-kb` | 500 | tamanho do PDF gerado |

O gerador também confere que o PDF baixado é **idêntico** ao assinado e que a validação aprova cada um.
