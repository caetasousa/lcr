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
| 📂 | **Documentos** guardados **cifrados**, visíveis para **todos que assinaram** | ✅ |
| 🚫 | **Revogar** o certificado e ver a LCR mudar | ✅ |
| 🔄 | **Renovar** (novo par de chaves e novo certificado) | ✅ |
| 💾 | **Exportar `.pfx`** (A1) para instalar no Windows ([o que é cada arquivo](#meu-certificado)) | ✅ |

---

<a id="meu-certificado"></a>
## 📁 Meu certificado: o que é cada arquivo e para que serve

Na tela **Certificado** aparecem estas opções:

| Opção | Arquivo | Tem a chave privada? | Pode compartilhar? | Serve para... |
|---|---|:---:|:---:|---|
| 📄 **Certificado em PEM** | (texto na tela) | ❌ | ✅ | ver ou copiar o certificado como texto |
| 📥 **Baixar .crt** | `meu_certificado.crt` | ❌ | ✅ | provar **quem você é**; outras pessoas conferem as suas assinaturas |
| 🔑 **Exportar .pfx (A1)** | `meu_certificado.pfx` | ✅ | 🚫 **nunca** | **assinar** fora do site: Adobe Reader, Word, e-mail |
| 🚫 **Revogar** | — | — | — | **cancelar** o certificado (por exemplo, se o `.pfx` vazar) |

> 💡 **Regra de ouro:** o `.crt` é como a **cópia do seu RG**: pode mandar para qualquer um. O `.pfx` é como o **seu RG mais a sua caneta**: quem tiver o arquivo e a senha assina **em seu nome**.

### 📄 Certificado em PEM

É o próprio certificado escrito em texto (Base64), entre as linhas `-----BEGIN CERTIFICATE-----` e `-----END CERTIFICATE-----`. O conteúdo é **o mesmo do `.crt`**, só que na tela.

* 📋 **Use para:** copiar e colar num e-mail, num relatório ou num site de inspeção de certificados.
* 🔍 Dá para inspecionar colando o texto num arquivo e rodando `openssl x509 -in arquivo.pem -noout -text`.

### 📥 Baixar .crt (`meu_certificado.crt`)

Contém **só a parte pública**: o seu nome e CPF, a sua chave pública, a validade e a assinatura da AC ACME. **Não** contém a chave privada, então **não serve para assinar**.

* 👀 **Ver os dados no Windows:** dê dois cliques no arquivo. Abre a janela *Certificado*, com as abas *Geral*, *Detalhes* e *Caminho de Certificação*.
* 📤 **Mandar para alguém** que queira conferir as suas assinaturas ou te mandar um e-mail cifrado.
* 🧪 **Conferir a cadeia** pelo terminal (veja a [Parte 2 da demonstração](docs/DEMONSTRACAO.md#parte-2)).
* ⚠️ **Não precisa instalar** o `.crt` para assinar. Se instalar, ele entra em *Outras Pessoas* e não habilita nada.

### 🔑 Exportar .pfx (A1) (`meu_certificado.pfx`)

É o pacote completo, no formato **PKCS#12**: o seu certificado, **a sua chave privada** e a cadeia da AC (intermediária e raiz), tudo cifrado com a **mesma senha da sua conta** (AES-256 + PBKDF2). É exatamente o que um **certificado A1** da ICP-Brasil entrega.

* 🔒 O site pede a senha de novo para exportar.
* 🚫 **Não compartilhe e não envie por e-mail.** Guarde num lugar seguro.
* ⏳ Só aparece enquanto o certificado está **válido** (não revogado e não vencido).

#### 💻 Instalar no Windows

1. Dê dois cliques no `meu_certificado.pfx`. Abre o *Assistente para Importação de Certificados*.
2. **Local do Repositório:** escolha **Usuário Atual** → *Avançar*.
3. Confirme o arquivo → *Avançar*.
4. **Senha:** digite a senha da sua conta no LCR Assina. Marque **"Marcar esta chave como exportável"** só se quiser poder exportá-la de novo depois.
5. **Repositório:** deixe *Selecionar automaticamente o repositório conforme o tipo de certificado* → *Avançar* → *Concluir*.
6. 🔎 **Para conferir:** `Win + R` → `certmgr.msc` → *Pessoal* → *Certificados*. Lá está `SEU NOME:CPF`, com o ícone de chave (indica que a chave privada veio junto).

> ⚠️ Para o Windows **confiar** no certificado (e não mostrar "identidade desconhecida"), instale também a **AC Raiz** (veja [Arquivos da AC ACME](#arquivos-ac), logo abaixo).

#### ✍️ O que dá para fazer com ele instalado

| Onde | Como |
|---|---|
| 📕 **Adobe Acrobat Reader** | *Todas as ferramentas* → *Usar um certificado* → *Assinar digitalmente* (na interface antiga: *Ferramentas* → *Certificados*) → desenhe o quadro → escolha `SEU NOME:CPF`. Gera uma assinatura PAdES, que o **Validar** deste site também reconhece. |
| 📘 **Word / Excel** (Windows) | *Arquivo* → *Informações* → *Proteger documento* → *Adicionar uma Assinatura Digital*. |
| ✉️ **Outlook / Thunderbird** | assinar e-mails (S/MIME). O certificado tem o uso *E-mail Protection*. |
| 🦊 **Firefox** | não usa o repositório do Windows: importe o `.pfx` em *Configurações* → *Privacidade e Segurança* → *Ver certificados* → *Seus certificados* → *Importar*. |
| 🐧 **Linux / macOS** | `openssl pkcs12 -in meu_certificado.pfx -info -noout` para inspecionar; no macOS, dois cliques importam no *Acesso às Chaves*. |

#### 🗑️ Remover do Windows

`certmgr.msc` → *Pessoal* → *Certificados* → clique com o botão direito em `SEU NOME:CPF` → *Excluir*.

### 🚫 Revogar

Cancela o certificado **antes do vencimento**. O número de série entra na **LCR** (Lista de Certificados Revogados), que qualquer validador consulta.

* 🔐 Pede a sua senha e um **motivo**:

  | Motivo | Quando usar |
  |---|---|
  | 🔓 Chave comprometida | o `.pfx` vazou, foi perdido ou alguém descobriu a senha |
  | 🔀 Mudança de vínculo com a ACME | você saiu da organização ou mudou de função |
  | 🔄 Substituído por outro certificado | você vai emitir um novo |
  | 🛑 Cessação de operação | não vai mais usar |
  | 🤷 Não especificado | nenhum dos anteriores |

* ⛔ **Não dá para desfazer.**
* ✍️ **Depois de revogar:**
  * não é mais possível **assinar** com ele, nem pelo site nem com o `.pfx` instalado (a assinatura sai reprovada na validação);
  * os PDFs assinados **antes** da revogação **continuam válidos**, com um aviso ⚠️;
  * o botão **Emitir novo certificado** aparece: ele gera um par de chaves novo e um certificado novo, com outro número de série;
  * se você instalou o `.pfx` antigo no Windows, **exclua-o** (veja acima) e instale o novo.

<a id="arquivos-ac"></a>
### 🏛️ Arquivos da AC ACME (menu **AC ACME**)

Esta página é **pública**: qualquer pessoa baixa estes arquivos, sem login.

| Arquivo | O que é | Instalar onde |
|---|---|---|
| `ac_acme_raiz_g2.crt` | 🏛️ certificado da **AC Raiz**: a âncora de confiança | ✅ **Autoridades de Certificação Raiz Confiáveis** |
| `ac_acme_assinatura.crt` | 🏢 certificado da **AC Intermediária**, que emite os usuários | opcional: *Autoridades de Certificação Intermediárias* (o `.pfx` já traz ela) |
| `ac_acme.crl` | 📜 a **LCR** atual, assinada pela intermediária | não se instala: serve para consultar (`openssl crl -inform DER -in ac_acme.crl -noout -text`) |

#### 💻 Instalar a AC Raiz no Windows

1. Dê dois cliques em `ac_acme_raiz_g2.crt` → **Instalar Certificado...**
2. **Usuário Atual** → *Avançar*.
3. Marque **Colocar todos os certificados no repositório a seguir** → *Procurar* → **Autoridades de Certificação Raiz Confiáveis** → *OK* → *Avançar* → *Concluir*.
4. O Windows mostra um **aviso de segurança** ("Você está prestes a instalar um certificado de uma autoridade de certificação..."). Confirme com **Sim**.
5. 🔎 Abra de novo o `meu_certificado.crt`: a aba *Caminho de Certificação* agora mostra **AC ACME Raiz G2 → AC ACME Assinatura Digital → SEU NOME:CPF**, com "Este certificado está OK".

> ⚠️ Instalar uma raiz faz o computador confiar em **tudo** o que essa AC assinar. Faça isso só em máquinas de teste ou de estudo. Para remover: `certmgr.msc` → *Autoridades de Certificação Raiz Confiáveis* → *Certificados* → `AC ACME Raiz G2` → *Excluir*.

> 📕 **Adobe Reader:** por padrão ele usa a lista de confiança da própria Adobe, não a do Windows. Para ele aceitar a AC ACME: *Editar* → *Preferências* → *Assinaturas* → *Verificação* → *Mais...* → em *Integração com o Windows*, marque **Validar assinaturas**.

---

## 🚀 Rodar localmente (um comando)

Pré-requisito: 🐳 **Docker** com o plugin **Compose**.

```bash
git clone https://github.com/caetasousa/lcr.git
cd lcr
docker compose up -d --build
```

Pronto: abra 👉 **http://localhost:8080**

> 🖱️ **Todo o uso normal é pelo navegador**: criar conta, assinar, validar, revogar, exportar. O terminal **não é necessário**. Ele só aparece na documentação como opção para conferir os resultados com o `openssl` (útil no relatório) ou para quem quiser integrar outro sistema via API.

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
* 📦 **Armazenamento:** cada PDF fica cifrado com **AES-256-GCM**, e a chave dele é cifrada com **RSA-OAEP** para cada signatário. Só quem assinou abre, e o arquivo devolvido é idêntico ao assinado (SHA-256 conferido).
* 🛡️ **Aplicação:** limites de tentativas, proteção contra CSRF (inclusive de subdomínios vizinhos), CSP, cookie `__Host-`, containers endurecidos e trilha de auditoria. Veja [docs/SEGURANCA.md](docs/SEGURANCA.md).
* 🚫 **Revogação:** LCR (lista de certificados revogados) assinada pela AC Intermediária, com os motivos da RFC 5280.

👉 **A explicação completa, passo a passo, está em [docs/CERTIFICADOS.md](docs/CERTIFICADOS.md).**

---

## 📚 Documentação

🧭 **Por onde começar?**

| Eu quero... | Leia |
|---|---|
| 🎬 **usar o app** e fazer a demonstração | [docs/DEMONSTRACAO.md](docs/DEMONSTRACAO.md): Parte 1 (navegador) e Parte 2 opcional (terminal) |
| 🔐 **entender a criptografia** (como o certificado e a assinatura funcionam) | [docs/CERTIFICADOS.md](docs/CERTIFICADOS.md): começa pelos conceitos básicos, com quadros "em palavras simples" |
| 🏗️ entender **como o código está organizado** | [docs/ARQUITETURA.md](docs/ARQUITETURA.md) |
| 🔌 chamar a API **de outro programa ou do terminal** | [docs/API.md](docs/API.md) |
| ☁️ **publicar no servidor** | [docs/DEPLOY.md](docs/DEPLOY.md) |
| 🛡️ ver **as defesas de segurança** e como testá-las | [docs/SEGURANCA.md](docs/SEGURANCA.md): força bruta, CSRF, CSP, uploads, containers, auditoria |
| 🏋️ saber **quantos acessos simultâneos aguenta** | [docs/CARGA.md](docs/CARGA.md): 60 usuários juntos, antes e depois dos limitadores |

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

🧪 Testes do backend e teste de ponta a ponta (com o compose no ar):

```bash
cd backend && go test -race ./...
bash scripts/e2e-documentos.sh   # armazenamento cifrado (24 itens)
bash scripts/e2e-seguranca.sh    # ataques e defesas (54 itens)
```

---

## ⚠️ Limitações (diferenças para o gov.br e a ICP-Brasil)

* 🪪 **Sem validação de identidade:** qualquer pessoa cria uma conta com um CPF válido.
* 🔧 **Sem HSM:** as chaves ficam no PostgreSQL, cifradas com a senha. Em produção real ficariam num HSM certificado (FIPS 140-2/3).
* ⏱️ **Sem carimbo de tempo** (ACT / RFC 3161): a data da assinatura é a declarada pelo servidor.
* 📦 **PAdES-B-B:** a LCR não vai dentro do PDF (níveis B-LT/B-LTA), então a validação de longo prazo depende de a AC continuar online.
* 🏛️ A AC ACME não faz parte da ICP-Brasil: no Adobe Reader a assinatura aparece como "identidade desconhecida" até a raiz ser instalada como confiável.
* 🔁 Não há segundo fator (2FA) nem recuperação de senha: quem esquece a senha perde o acesso à chave privada.

> 📌 Projeto didático, **sem validade jurídica**.
