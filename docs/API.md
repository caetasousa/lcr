# 🔌 API REST

Base: `http://localhost:8080` (local) ou `https://lcr.d4ffy.com` (produção).

🍪 A autenticação é por **cookie de sessão** (`HttpOnly; SameSite=Strict`), criado no login. Erros voltam como JSON `{"erro": "mensagem"}`.

## 🔓 Públicas

| Método | Rota | Corpo | Descrição |
|---|---|---|---|
| `POST` | `/api/cadastro` | JSON `{nome, cpf, senha}` | 🪪 cria a conta e a AC emite o certificado |
| `POST` | `/api/login` | JSON `{cpf, senha}` | 🔑 abre a sessão (cookie) |
| `POST` | `/api/logout` | — | 🚪 encerra a sessão |
| `POST` | `/api/validar` | multipart `arquivo` | 🔍 valida o PDF e devolve o resultado em JSON |
| `GET` | `/api/ac` | — | 🏛️ dados da AC Raiz e da Intermediária |
| `GET` | `/api/ac/raiz.crt` | — | 📥 certificado da AC Raiz (PEM) |
| `GET` | `/api/ac/intermediaria.crt` | — | 📥 certificado da AC Intermediária (PEM) |
| `GET` | `/api/ac/lcr.crl` | — | 📜 LCR atual (DER) |

## 🔒 Autenticadas

| Método | Rota | Corpo | Descrição |
|---|---|---|---|
| `GET` | `/api/eu` | — | 👤 usuário logado e o certificado dele |
| `POST` | `/api/assinar` | multipart: `arquivo`, `pagina`, `x1`, `y1`, `x2`, `y2` (pontos PDF), `senha`, `motivo` | 🖋️ assina e devolve o PDF |
| `GET` | `/api/documentos` | — | 📂 histórico de documentos assinados |
| `GET` | `/api/documentos/{id}` | — | 📥 baixa um PDF assinado |
| `GET` | `/api/certificado.crt` | — | 📥 certificado do usuário (PEM) |
| `POST` | `/api/certificado/pfx` | JSON `{senha}` | 💾 PKCS#12 (AES-256-CBC + PBKDF2-SHA256) |
| `POST` | `/api/certificado/revogar` | JSON `{senha, motivo}` | 🚫 revoga (motivo = código CRLReason: 0, 1, 3, 4, 5) |
| `POST` | `/api/certificado/renovar` | JSON `{senha}` | 🔄 novo par de chaves e novo certificado |

📏 PDFs: até **20 MiB**, e o arquivo precisa começar com `%PDF-`.

## 🧪 Exemplos com `curl`

```bash
# 🏛️ baixar a cadeia da AC
curl -s localhost:8080/api/ac/raiz.crt -o raiz.crt
curl -s localhost:8080/api/ac/intermediaria.crt -o inter.crt

# 🪪 criar conta e logar (guardando o cookie)
curl -s -c cookies.txt localhost:8080/api/cadastro -H 'Content-Type: application/json' \
  -d '{"nome":"Maria Silva","cpf":"529.982.247-25","senha":"senha-forte-123"}'
curl -s -c cookies.txt localhost:8080/api/login -H 'Content-Type: application/json' \
  -d '{"cpf":"529.982.247-25","senha":"senha-forte-123"}'

# 🖋️ assinar a página 1, carimbo no canto inferior esquerdo
curl -s -b cookies.txt localhost:8080/api/assinar \
  -F arquivo=@contrato.pdf -F pagina=1 -F x1=40 -F y1=40 -F x2=260 -F y2=110 \
  -F senha=senha-forte-123 -F motivo="Aprovação" -o contrato_assinado.pdf

# 🔍 validar
curl -s localhost:8080/api/validar -F arquivo=@contrato_assinado.pdf
```
