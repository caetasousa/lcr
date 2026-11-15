import { useEffect, useState } from 'react'
import { api, dataHora, type InfoAC } from '../api'
import Icone from '../components/Icone'

function CartaoAC({ rotulo, d, link, i }: { rotulo: string; d: Record<string, string>; link: string; i: number }) {
  return (
    <div className="painel revelar" style={{ '--i': i } as React.CSSProperties}>
      <div className="painel-cabecalho">
        <div>
          <span className="rotulo">{rotulo}</span>
          <h3 className="subtitulo" style={{ marginTop: 4 }}>{d.titular.match(/CN=([^,]+)/)?.[1] ?? d.titular}</h3>
        </div>
        <a className="botao" href={link}><Icone nome="baixar" tamanho={14} /> .crt</a>
      </div>
      <dl className="detalhes">
        <dt>Emissor</dt><dd><code>{d.emissor}</code></dd>
        <dt>Série</dt><dd><code>{d.serial}</code></dd>
        <dt>Validade</dt><dd>{dataHora(d.valido_de)} → {dataHora(d.valido_ate)}</dd>
        <dt>Algoritmo</dt><dd>{d.algoritmo}</dd>
        <dt>SHA-256</dt><dd><code>{d.fingerprint_sha256}</code></dd>
      </dl>
    </div>
  )
}

export default function AutoridadeCertificadora() {
  const [ac, setAc] = useState<InfoAC | null>(null)
  const [erro, setErro] = useState('')
  useEffect(() => {
    api.ac().then(setAc).catch((e) => setErro(e.message))
  }, [])

  return (
    <div>
      <header className="cabecalho-pagina revelar">
        <span className="rotulo">Infraestrutura de chaves públicas</span>
        <h1 className="titulo">Autoridade Certificadora ACME</h1>
        <p>
          Mesmo modelo da ICP-Brasil: a raiz só assina a AC intermediária e fica "offline", ou seja, a chave dela não é
          carregada na memória do servidor. A intermediária emite os certificados dos usuários e a LCR.
        </p>
      </header>

      <div className="painel revelar" style={{ '--i': 1 } as React.CSSProperties}>
        <span className="rotulo">Hierarquia</span>
        <ul className="arvore">
          <li><div className="no"><b>AC ACME Raiz G2</b><span>autoassinada · RSA-4096 · 10 anos · Certificate Sign, CRL Sign</span></div></li>
          <li><div className="no"><b>AC ACME Assinatura Digital</b><span>RSA-3072 · 5 anos · pathLen 0 · emite usuários e a LCR</span></div></li>
          <li><div className="no"><b>NOME DO USUÁRIO:CPF</b><span>RSA-3072 · 1 ano · Digital Signature, Non Repudiation, Document Signing</span></div></li>
        </ul>
      </div>

      {erro && <p className="erro">{erro}</p>}
      {ac && (
        <>
          <div className="grade2" style={{ marginTop: 'var(--e3)' }}>
            <CartaoAC rotulo="AC Raiz" d={ac.raiz} link="/api/ac/raiz.crt" i={2} />
            <CartaoAC rotulo="AC Intermediária" d={ac.intermediaria} link="/api/ac/intermediaria.crt" i={3} />
          </div>

          <div className="painel revelar" style={{ '--i': 4, marginTop: 'var(--e3)' } as React.CSSProperties}>
            <div className="painel-cabecalho">
              <div>
                <span className="rotulo">LCR · RFC 5280</span>
                <h3 className="subtitulo" style={{ marginTop: 4 }}>Lista de Certificados Revogados</h3>
              </div>
              <a className="botao" href="/api/ac/lcr.crl"><Icone nome="baixar" tamanho={14} /> ac_acme.crl</a>
            </div>
            {ac.revogados.length === 0 ? (
              <p className="sutil">Nenhum certificado revogado.</p>
            ) : (
              <div className="tabela-wrap" style={{ marginBottom: 0 }}>
                <table>
                  <thead><tr><th>Série</th><th>Titular</th><th>Revogado em</th><th>Motivo</th></tr></thead>
                  <tbody>
                    {ac.revogados.map((r) => (
                      <tr key={r.serial}>
                        <td><code>{r.serial}</code></td>
                        <td>{r.titular}</td>
                        <td className="sutil">{dataHora(r.revogado_em)}</td>
                        <td><span className="chip falha">{r.motivo}</span></td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
            <pre className="diagrama mono" style={{ marginTop: 'var(--e3)', marginBottom: 0 }}>
              <span className="fraco">$</span> openssl crl -inform DER -in ac_acme.crl -noout -text
            </pre>
          </div>

          <div className="nota revelar" style={{ '--i': 5, marginTop: 'var(--e3)' } as React.CSSProperties}>
            <Icone nome="escudo" />
            <span>
              Para o Windows ou o Adobe confiarem nas assinaturas, instale a <b>AC Raiz</b> em "Autoridades de Certificação
              Raiz Confiáveis" (item 4.2 do laboratório). A AC ACME não faz parte da ICP-Brasil.
            </span>
          </div>
        </>
      )}
    </div>
  )
}
