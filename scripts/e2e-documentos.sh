#!/usr/bin/env bash
# 🧪 Teste de ponta a ponta do armazenamento cifrado dos documentos.
# Usa a API e o PostgreSQL reais do docker compose local.
#
#   docker compose up -d --build
#   bash scripts/e2e-documentos.sh
#
# Cria três contas novas (Ana, Beto, Carla), assina, baixa, adultera o banco
# e confere tudo. Os arquivos de trabalho ficam numa pasta temporária.
set -u
B=${B:-http://localhost:8080}
TRABALHO=$(mktemp -d); cd "$TRABALHO"; echo "📁 arquivos em $TRABALHO"
ok=0; falha=0
check() { if [ "$1" = "$2" ]; then echo "  ✅ $3"; ok=$((ok+1)); else echo "  ❌ $3 (esperado: $2, obtido: $1)"; falha=$((falha+1)); fi; }
cpf() { python3 -c "
import random
n=[random.randint(0,9) for _ in range(9)]
for k in (10,11):
    s=sum(d*(k-i) for i,d in enumerate(n)); r=(s*10)%11; n.append(0 if r==10 else r)
print(''.join(map(str,n)))"; }
conta() { # nome cpf senha cookie
  curl -s -o /dev/null $B/api/cadastro -H 'Content-Type: application/json' -d "{\"nome\":\"$1\",\"cpf\":\"$2\",\"senha\":\"$3\"}"
  curl -s -o /dev/null -c $4 $B/api/login -H 'Content-Type: application/json' -d "{\"cpf\":\"$2\",\"senha\":\"$3\"}"; }
assina() { # cookie entrada saida senha y -> imprime id
  curl -s -D h.txt -b $1 $B/api/assinar -F arquivo=@$2 -F pagina=1 -F x1=40 -F y1=$5 -F x2=300 -F y2=$(($5+66)) -F senha=$4 -o $3
  grep -i x-documento-id h.txt | tr -dc 0-9; }
baixa() { curl -s -o $4 -w "%{http_code}" -b $1 $B/api/documentos/$2 -H 'Content-Type: application/json' -d "{\"senha\":\"$3\"}"; }
sha() { sha256sum "$1" | cut -c1-64; }
db() { docker exec lcr-db-1 psql -U acme -d acme -tAc "$1"; }

python3 - <<'PY'
objs=[b"<< /Type /Catalog /Pages 2 0 R >>", b"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>"]
st=b"BT /F1 24 Tf 72 760 Td (Contrato E2E) Tj ET"
objs.append(b"<< /Length %d >>\nstream\n"%len(st)+st+b"\nendstream")
objs.append(b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
out=b"%PDF-1.7\n"; offs=[]
for i,o in enumerate(objs,1):
    offs.append(len(out)); out+=b"%d 0 obj\n"%i+o+b"\nendobj\n"
x=len(out); out+=b"xref\n0 %d\n0000000000 65535 f \n"%(len(objs)+1)
for o in offs: out+=b"%010d 00000 n \n"%o
out+=b"trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n"%(len(objs)+1,x)
open("original.pdf","wb").write(out)
PY

echo "🪪 1. Contas"
conta "Ana Teste" $(cpf) senha-ana-123 ana.txt; conta "Beto Teste" $(cpf) senha-beto-123 beto.txt; conta "Carla Teste" $(cpf) senha-carla-123 carla.txt
check "$(curl -s -b ana.txt $B/api/eu | python3 -c 'import json,sys;print(json.load(sys.stdin)["nome"])')" "Ana Teste" "Ana logada"

echo "🖋️ 2. Ana assina; Beto assina o PDF da Ana"
V1=$(assina ana.txt original.pdf v1.pdf senha-ana-123 40)
V2=$(assina beto.txt v1.pdf v2.pdf senha-beto-123 140)
check "$( [ -n "$V1" ] && [ -n "$V2" ] && echo sim)" "sim" "duas versões gravadas (v1=$V1, v2=$V2)"

echo "🔐 3. No banco fica só o cifrado"
check "$(db "select substring(pdf_assinado from 1 for 4)='%PDF' from documentos where id in ($V1,$V2)" | sort -u)" "f" "nenhum PDF em claro no banco"
check "$(db "select count(*) from documento_chaves where documento_id=$V2")" "2" "v2 tem 2 chaves de acesso (Ana e Beto)"
check "$(db "select count(*) from documento_chaves where documento_id=$V1")" "1" "v1 tem 1 chave de acesso (Ana)"

echo "👥 4. Quem vê o quê"
lista() { curl -s -b $1 $B/api/documentos | python3 -c "import json,sys; d={x['id']:x for x in json.load(sys.stdin)}; v=d.get($2); print(' + '.join(v['signatarios']) if v else '-')"; }
check "$(lista ana.txt $V2)" "Ana Teste + Beto Teste" "Ana vê v2 com os dois signatários"
check "$(lista beto.txt $V2)" "Ana Teste + Beto Teste" "Beto vê v2 com os dois signatários"
check "$(lista beto.txt $V1)" "-" "Beto não vê v1 (não assinou)"
check "$(lista carla.txt $V2)" "-" "Carla não vê v2 (não assinou)"

echo "📥 5. Download decifrado é idêntico byte a byte"
check "$(baixa ana.txt $V2 senha-ana-123 ana_v2.pdf)" "200" "Ana baixa v2 com a senha"
check "$(baixa beto.txt $V2 senha-beto-123 beto_v2.pdf)" "200" "Beto baixa v2 com a senha"
check "$(sha ana_v2.pdf)" "$(sha v2.pdf)" "arquivo da Ana == arquivo devolvido na assinatura"
check "$(sha beto_v2.pdf)" "$(sha v2.pdf)" "arquivo do Beto == arquivo devolvido na assinatura"
check "$(sha ana_v2.pdf)" "$(db "select sha256_assinado from documentos where id=$V2")" "SHA-256 == registrado no banco"
check "$(cmp -s <(head -c $(stat -c%s original.pdf) ana_v2.pdf) original.pdf && echo igual)" "igual" "original preservado intacto dentro do assinado"

echo "🚫 6. Acessos negados"
check "$(baixa ana.txt $V2 senha-errada x.pdf)" "403" "senha errada → 403"
check "$(baixa carla.txt $V2 senha-carla-123 x.pdf)" "404" "Carla (não signatária) → 404"
check "$(curl -s -o /dev/null -w '%{http_code}' $B/api/documentos/$V2 -H 'Content-Type: application/json' -d '{"senha":"x"}')" "401" "sem login → 401"
check "$(curl -s -o /dev/null -w '%{http_code}' -b ana.txt $B/api/documentos/$V2)" "404" "GET antigo (sem senha) não existe mais → 404"

echo "🔍 7. O baixado continua válido"
check "$(curl -s $B/api/validar -F arquivo=@ana_v2.pdf | python3 -c 'import json,sys;r=json.load(sys.stdin);print(r["valido"], len(r["assinaturas"]))')" "True 2" "validação: 2 assinaturas válidas"

echo "🧨 8. Adulteração no banco é detectada e o arquivo NÃO é entregue"
db "update documentos set pdf_assinado = overlay(pdf_assinado placing decode(lpad(to_hex(get_byte(pdf_assinado,500) # 1),2,'0'),'hex') from 501 for 1) where id=$V2" >/dev/null
check "$(baixa ana.txt $V2 senha-ana-123 adulterado.pdf)" "422" "conteúdo adulterado → 422"
check "$(head -c 4 adulterado.pdf)" "{\"er" "resposta é erro JSON, não PDF"
db "update documentos set pdf_assinado = overlay(pdf_assinado placing decode(lpad(to_hex(get_byte(pdf_assinado,500) # 1),2,'0'),'hex') from 501 for 1) where id=$V2" >/dev/null
check "$(baixa ana.txt $V2 senha-ana-123 restaurado.pdf)" "200" "byte restaurado → volta a abrir"

echo "📦 9. Migração dos antigos"
check "$(db "select count(*) from documentos where not cifrado")" "0" "nenhum documento antigo em claro"

echo; echo "Resultado: $ok passaram, $falha falharam"; [ $falha -eq 0 ]
