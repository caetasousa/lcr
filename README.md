# ✍️ LCR Assina — assinatura digital de PDF no estilo gov.br

> 🎓 Projeto didático da disciplina de **Criptografia**. Uma Autoridade Certificadora (AC) própria, escrita em **Go**, emite certificados X.509 para os usuários, que assinam PDFs no padrão **PAdES**, e qualquer pessoa pode validar o documento, como no validar.iti.gov.br.

🌐 **Em produção:** https://lcr.d4ffy.com

É a continuação do *Laboratório de Certificado Digital – Empresa ACME*: a AC e os certificados que lá eram criados à mão com `openssl` agora são gerados por um servidor.

---

## ✨ O que dá para fazer

| | Funcionalidade | Precisa de login? |
|---|---|:---:|
| 🪪 | **Criar conta**: a AC ACME emite na hora um certificado X.509 com o seu nome e CPF | — |
| 🖋️ | **Assinar PDF**: escolher a página e o ponto exato do carimbo e assinar com a sua chave | ✅ |
| 🔍 | **Validar PDF**: integridade, autenticidade, cadeia e revogação | ❌ |
| 🏛️ | **Ver a AC ACME**: baixar a AC Raiz, a AC Intermediária e a LCR | ❌ |
| 📂 | **Histórico** de documentos assinados | ✅ |
| 🚫 | **Revogar** o certificado e ver a LCR mudar | ✅ |
| 🔄 | **Renovar** (novo par de chaves e novo certificado) | ✅ |
| 💾 | **Exportar `.pfx`** (A1) para instalar no Windows | ✅ |

---

## 🚀 Rodar localmente (um comando)

Pré-requisito: 🐳 **Docker** com o plugin **Compose**.

```bash
git clone https://github.com/caetasousa/lcr.git
cd lcr
docker compose up -d --build
```

Pronto: abra 👉 **http://localhost:8080**

* 🔑 **Não precisa criar `.env`.** O `docker-compose.yml` já traz como padrão as credenciais de desenvolvimento que estão no [`.env.example`](.env.example). Para mudá-las, copie com `cp .env.example .env` e edite.
* ⏳ Na primeira execução o servidor cria a AC Raiz (RSA-4096) e a AC Intermediária (RSA-3072). Isso leva alguns segundos.
* 🧪 CPFs de teste válidos: `529.982.247-25` e `111.444.777-35`.

| Comando | Para quê |
|---|---|
| `docker compose logs -f app` | 📜 acompanhar os logs |
| `docker compose down` | ⏹️ parar (os dados ficam no volume) |
| `docker compose down -v` | 🧨 parar **e apagar** banco e AC (recomeça do zero) |

> ⚠️ As credenciais do `.env.example` são **só para uso local**. A produção usa senhas aleatórias diferentes, geradas pelo script de deploy.

---

## 🧰 Tecnologias utilizadas

### ⚙️ Backend (Go 1.27)

| Biblioteca | Uso |
|---|---|
| `crypto/x509`, `crypto/rsa`, `crypto/sha256` (stdlib) | 🏛️ AC: emissão de certificados, LCR, chaves RSA |
| `crypto/aes` + `crypto/cipher` (GCM) (stdlib) | 🔒 cifrar as chaves privadas guardadas no banco |
| `golang.org/x/crypto/argon2` | 🧂 hash de senha e derivação de chave (Argon2id) |
| `github.com/digitorus/pdfsign` | 🖋️ assinatura e validação PAdES (CMS dentro do PDF) |
| `software.sslmate.com/src/go-pkcs12` | 💾 exportação `.pfx` (PKCS#12) |
| `github.com/jackc/pgx/v5` | 🐘 acesso ao PostgreSQL |
| `golang.org/x/text` | 🔤 texto do carimbo em WinAnsi (acentos) |

### 🎨 Frontend

| Biblioteca | Uso |
|---|---|
| ⚛️ React 19 + TypeScript + Vite | interface (SPA) |
| 📄 pdf.js (`pdfjs-dist`) | mostrar o PDF e posicionar o carimbo |
| 🌌 Three.js | fundo animado em WebGL (com fallback em CSS) |

### 🏗️ Infraestrutura

| Peça | Uso |
|---|---|
| 🐳 Docker multi-stage | `node:26-alpine` → `golang:1.27-alpine` → `alpine:3.24` (imagem final pequena) |
| 🐘 PostgreSQL 18 | usuários, certificados, chaves cifradas, sessões, PDFs assinados |
| ☁️ Cloudflare Tunnel | publica o app em produção sem abrir portas |

---

## 🔐 Criptografia em 30 segundos

```
🏛️ AC ACME Raiz G2              RSA-4096 · 10 anos · autoassinada · chave "offline"
 └── 🏢 AC ACME Assinatura Digital  RSA-3072 · 5 anos  · emite usuários e a LCR
      └── 👤 NOME:CPF                RSA-3072 · 1 ano   · assina documentos
```

* ✍️ **Assinatura:** SHA-256 do PDF → assinado com **RSA PKCS#1 v1.5** → envelope **CMS** (PAdES-B-B) gravado dentro do próprio PDF.
* 🔑 **Chave privada do usuário:** fica no servidor (como no gov.br), **cifrada com a senha dele**: Argon2id deriva a chave e o AES-256-GCM cifra.
* 🚫 **Revogação:** LCR (lista de certificados revogados) assinada pela AC Intermediária, com os motivos da RFC 5280.

👉 **A explicação completa, passo a passo, está em [docs/CERTIFICADOS.md](docs/CERTIFICADOS.md).**

---

## 📚 Documentação

| Documento | Conteúdo |
|---|---|
| 🔐 [docs/CERTIFICADOS.md](docs/CERTIFICADOS.md) | **Como funciona o algoritmo**: hierarquia da AC, emissão, cofre de chaves, assinatura PAdES, validação e LCR |
| 🏗️ [docs/ARQUITETURA.md](docs/ARQUITETURA.md) | Arquitetura hexagonal do backend, frontend e banco de dados |
| 🔌 [docs/API.md](docs/API.md) | Rotas REST |
| 🎬 [docs/DEMONSTRACAO.md](docs/DEMONSTRACAO.md) | Roteiro de demonstração e comandos `openssl` para o relatório |
| ☁️ [docs/DEPLOY.md](docs/DEPLOY.md) | Instalação no servidor (Docker rootless + Cloudflare Tunnel) |

---

## 🗂️ Estrutura do repositório

```
lcr/
├── 📁 backend/            API em Go (arquitetura hexagonal)
├── 📁 frontend/           SPA React + Vite
├── 📁 deploy/             compose de produção e script de instalação
├── 📁 docs/               documentação detalhada
├── 🐳 Dockerfile          build multi-stage (front + API numa imagem só)
├── 🐳 docker-compose.yml  app + PostgreSQL (pronto para rodar local)
└── 🔧 .env.example        credenciais de desenvolvimento
```

---

## 🧑‍💻 Desenvolver sem Docker

```bash
docker compose up -d db                                      # 🐘 só o banco (publique a 5432 se precisar)
cd backend  && AC_SENHA=lcr-local-dev-ac-2026 go run ./cmd/server   # ⚙️ API na :8080
cd frontend && npm install && npm run dev                    # 🎨 Vite (redireciona /api para a :8080)
```

🧪 Testes do backend:

```bash
cd backend && go test -race ./...
```

---

## ⚠️ Limitações (diferenças para o gov.br e a ICP-Brasil)

* 🪪 **Sem validação de identidade:** qualquer pessoa cria uma conta com um CPF válido.
* 🔧 **Sem HSM:** as chaves ficam no PostgreSQL, cifradas com a senha. Em produção real ficariam num HSM certificado (FIPS 140-2/3).
* ⏱️ **Sem carimbo de tempo** (ACT / RFC 3161): a data da assinatura é a declarada pelo servidor.
* 📦 **PAdES-B-B:** a LCR não vai dentro do PDF (níveis B-LT/B-LTA), então a validação de longo prazo depende de a AC continuar online.
* 🏛️ A AC ACME não faz parte da ICP-Brasil: no Adobe Reader a assinatura aparece como "identidade desconhecida" até a raiz ser instalada como confiável.
* 🛑 Não há limite de tentativas de login além do custo do Argon2id.

> 📌 Projeto didático, **sem validade jurídica**.
