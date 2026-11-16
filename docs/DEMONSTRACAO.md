# 🎬 Demonstração passo a passo

Este roteiro tem **duas partes independentes**:

| Parte | Para quem | Precisa de quê |
|---|---|---|
| 🖱️ **[Parte 1: usando o app](#parte-1)** | qualquer pessoa; é a demonstração principal | só o navegador |
| 💻 **[Parte 2: conferindo pelo terminal](#parte-2)** | quem quer provar no relatório que a criptografia é real | terminal com `curl` e `openssl` |

> 💡 **Faça a Parte 1 primeiro.** A Parte 2 é opcional: ela usa os arquivos baixados na Parte 1 para conferir, com ferramentas independentes, que os certificados e assinaturas seguem os padrões.

**Antes de começar**, suba o projeto e abra o app:

```bash
docker compose up -d --build
```

👉 **http://localhost:8080** (ou **https://lcr.d4ffy.com**, em produção)

Tenha à mão **um PDF qualquer** (um contrato, um trabalho, qualquer coisa).

---

<a id="parte-1"></a>
## 🖱️ Parte 1: usando o app no navegador

### 1️⃣ Criar a conta e receber o certificado

1. Na tela inicial, preencha **Nome completo**, **CPF** e **Senha** (mínimo 8 caracteres) e confirme a senha.
2. Use um CPF de teste: `529.982.247-25` ou `111.444.777-35`.
3. Clique em **Criar conta e emitir certificado**.

✨ **O que aconteceu:** o servidor gerou um par de chaves RSA-3072 só seu, e a AC ACME emitiu um certificado com o seu nome e CPF. A chave privada foi guardada **cifrada com a sua senha**.

📸 **Print:** abra o menu **Certificado**. Aparecem o titular (`SEU NOME:CPF`), o emissor, o número de série, a validade e os usos permitidos.

### 2️⃣ Assinar um PDF

1. Menu **Assinar** → envie o PDF.
2. **Clique na página** onde o carimbo deve aparecer. Dá para arrastar e redimensionar o quadro.
3. Digite **Sua senha** (e, se quiser, um **Motivo**, como "Aceite do contrato").
4. Clique em **Assinar documento** e baixe o arquivo `..._assinado.pdf`.

✨ **O que aconteceu:** a sua senha destrancou a chave privada só durante a assinatura. O servidor calculou o SHA-256 do PDF, assinou esse hash com a sua chave e gravou a assinatura **dentro do próprio PDF**.

📸 **Print:** o PDF aberto com o carimbo visível.

### 3️⃣ Validar o PDF assinado ✅

1. Menu **Validar** (não precisa estar logado) → envie o `..._assinado.pdf`.

✅ **Resultado esperado:** *Todas as assinaturas são válidas*, com as verificações **Documento íntegro**, **Cadeia de certificação confiável** e **Certificado não revogado**, além da cadeia Raiz → Intermediária → você.

📸 **Print:** a tela de validação aprovada.

### 4️⃣ Alterar o PDF e validar de novo ❌

Aqui você mostra que a assinatura **detecta qualquer alteração**.

1. Faça uma cópia do `..._assinado.pdf` e mude **um único caractere** dentro dela, com um editor hexadecimal (como o HxD) ou com o Bloco 8 do laboratório.
2. Valide a cópia alterada em **Validar**.

❌ **Resultado esperado:** *Documento alterado após a assinatura*. O SHA-256 recalculado não bate com o que foi assinado (`Message digest mismatch`).

📸 **Print:** a tela de validação reprovada.

### 5️⃣ Duas pessoas assinando o mesmo documento 👥

1. Clique em sair (ícone ao lado do seu nome) e **crie uma segunda conta** com o outro CPF de teste.
2. Em **Assinar**, envie o PDF **que já foi assinado** pela primeira pessoa e assine de novo, com o carimbo em outro lugar.
3. Valide o resultado.

✅ **Resultado esperado:** a validação mostra **duas assinaturas**, ambas válidas.

### 6️⃣ Revogar o certificado 🚫

1. Entre com a primeira conta → menu **Certificado** → **Revogar**.
2. Escolha um motivo (por exemplo, *Chave comprometida*) e confirme com a senha.
3. Valide de novo o PDF assinado no passo 2.
4. Tente assinar um documento novo.

✅ **Resultados esperados:**
* O PDF antigo **continua válido**, com o aviso ⚠️ *Certificado revogado depois da assinatura*. Quando ele foi assinado, o certificado estava bom.
* Assinar um documento novo é **bloqueado**.

### 7️⃣ Ver a LCR (Lista de Certificados Revogados) 📜

1. Menu **AC ACME** (é público).
2. Na seção **Lista de Certificados Revogados** aparece o número de série que você revogou, com a data e o motivo.
3. Clique em **ac_acme.crl** para baixar a lista oficial assinada pela AC (ela é usada na Parte 2).

### 8️⃣ Emitir um certificado novo e exportar o `.pfx` 💾

> 📖 O passo a passo detalhado (o que é cada arquivo, instalação no Windows, uso no Adobe Reader e no Word) está no [README, seção Meu certificado](../README.md#meu-certificado).

1. Menu **Certificado** → **Emitir novo certificado** (só aparece depois de revogar ou expirar). Confirme com a senha.
2. Clique em **Exportar .pfx (A1)** e confirme com a senha.
3. No Windows, dê dois cliques no `.pfx` para instalar (como no item 4.3 do laboratório). A senha do `.pfx` é a mesma da conta.
4. No menu **AC ACME**, baixe o **.crt da AC Raiz** e instale em *Autoridades de Certificação Raiz Confiáveis*. Sem isso, o Windows não reconhece a AC ACME.

📸 **Print:** o certificado instalado no Windows, mostrando a cadeia completa.

---

<a id="parte-2"></a>
## 💻 Parte 2: conferindo pelo terminal (opcional)

Aqui você confere, **fora do app**, que tudo segue os padrões. Use o `openssl`, que não tem nada a ver com o projeto. Funciona no Linux, no WSL ou no Git Bash.

📁 Rode tudo **na mesma pasta** onde estão os arquivos baixados na Parte 1:

| Arquivo | De onde veio |
|---|---|
| `meu_certificado.crt` | menu **Certificado** → **Baixar .crt** |
| `contrato_assinado.pdf` | passo 2 (troque pelo nome do seu arquivo) |
| `meu_certificado.pfx` | passo 8 |

> 🌐 Os comandos usam `localhost:8080`. Em produção, troque por `https://lcr.d4ffy.com`.

### 🅰️ Baixar e inspecionar a cadeia da AC

```bash
curl -s localhost:8080/api/ac/raiz.crt          -o raiz.crt
curl -s localhost:8080/api/ac/intermediaria.crt -o inter.crt

openssl x509 -in raiz.crt  -noout -subject -issuer -dates   # 🏛️ raiz: emissor = ela mesma (autoassinada)
openssl x509 -in inter.crt -noout -subject -issuer -dates   # 🏢 intermediária: emissor = raiz
```

### 🅱️ Verificar a cadeia do seu certificado

```bash
openssl x509 -in meu_certificado.crt -noout -text   # 👤 todos os campos: CN=NOME:CPF, Key Usage, LCR...
openssl verify -CAfile raiz.crt -untrusted inter.crt meu_certificado.crt
```

✅ Esperado: `meu_certificado.crt: OK`. Ou seja, o OpenSSL também reconhece que **raiz → intermediária → você** é uma cadeia válida.

### 🅲 Ler a LCR

```bash
curl -s localhost:8080/api/ac/lcr.crl -o ac_acme.crl
openssl crl -inform DER -in ac_acme.crl -noout -text
```

✅ Esperado: o *Issuer* é a AC intermediária e, depois do passo 6, aparece o seu número de série em *Revoked Certificates*, com o motivo.

### 🅳 Ver a assinatura escondida dentro do PDF

A assinatura fica no campo `/Contents` do PDF, em hexadecimal. Este script a extrai para um arquivo `.p7s` (CMS):

```bash
python3 - contrato_assinado.pdf <<'EOF'
import re, sys
dados = open(sys.argv[1], 'rb').read()
hexa = re.search(rb'/Contents\s*<([0-9A-Fa-f]+)>', dados).group(1)
open('assinatura.p7s', 'wb').write(bytes.fromhex(hexa.decode()).rstrip(b'\0'))
print('assinatura.p7s criado')
EOF

openssl cms -cmsout -inform DER -in assinatura.p7s -print -noout | less
```

✅ Esperado: aparecem o `messageDigest` (o SHA-256 do documento), o `SigningCertificateV2` e os **três certificados** da cadeia. Isso prova que a cadeia inteira viaja **dentro** do PDF.

### 🅴 Inspecionar o `.pfx`

```bash
openssl pkcs12 -in meu_certificado.pfx -info -noout
```

✅ Esperado: pede a senha e mostra `PBES2, PBKDF2, AES-256-CBC`, a cifra moderna usada para proteger a chave.
