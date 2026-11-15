import { useEffect, useState } from 'react'
import { api, baixar, dataHora, type Documento } from '../api'
import Icone from '../components/Icone'
import { navegar } from '../App'

export default function Documentos({ onValidar }: { onValidar: (b: Blob, nome: string) => void }) {
  const [docs, setDocs] = useState<Documento[] | null>(null)
  const [erro, setErro] = useState('')

  useEffect(() => {
    api.documentos().then(setDocs).catch((e) => setErro(e.message))
  }, [])

  const acao = async (d: Documento, validar: boolean) => {
    try {
      const b = await api.documento(d.id)
      if (validar) onValidar(b, d.nome)
      else baixar(b, d.nome)
    } catch (e) {
      setErro((e as Error).message)
    }
  }

  return (
    <div>
      <header className="cabecalho-pagina revelar">
        <span className="rotulo">Histórico</span>
        <h1 className="titulo">Meus documentos</h1>
      </header>
      {erro && <p className="erro">{erro}</p>}
      {!docs && !erro && <div className="carregando"><span className="giro" /> Carregando…</div>}

      {docs && docs.length === 0 && (
        <div className="painel vazio revelar">
          <div className="icone-grande"><Icone nome="documento" tamanho={24} /></div>
          <p className="subtitulo">Nenhum documento assinado ainda</p>
          <p className="sutil">Os PDFs que você assinar ficam guardados aqui.</p>
          <button className="primario grande" onClick={() => navegar('/assinar')}><Icone nome="caneta" tamanho={16} /> Assinar um documento</button>
        </div>
      )}

      {docs && docs.length > 0 && (
        <div className="painel revelar" style={{ '--i': 1 } as React.CSSProperties}>
          <div className="painel-cabecalho">
            <span className="sutil">{docs.length} documento(s)</span>
            <button className="primario" onClick={() => navegar('/assinar')}><Icone nome="caneta" tamanho={14} /> Novo</button>
          </div>
          <div className="tabela-wrap">
            <table>
              <thead>
                <tr>
                  <th>Documento</th>
                  <th>Assinado em</th>
                  <th>Pág.</th>
                  <th>SHA-256</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {docs.map((d) => (
                  <tr key={d.id}>
                    <td>
                      <div className="doc-cabecalho">
                        <div className="doc-icone"><Icone nome="documento" tamanho={16} /></div>
                        <div>
                          <div className="nome">{d.nome}</div>
                          <div className="fraco" style={{ fontSize: 12 }}>{(d.tamanho / 1024).toFixed(0)} KB</div>
                        </div>
                      </div>
                    </td>
                    <td className="sutil">{dataHora(d.assinado_em)}</td>
                    <td className="sutil">{d.pagina}</td>
                    <td><code title={d.sha256_assinado}>{d.sha256_assinado.slice(0, 12)}…</code></td>
                    <td className="acoes-linha">
                      <div className="acoes direita">
                        <button onClick={() => acao(d, true)}><Icone nome="escudo" tamanho={14} /> Validar</button>
                        <button onClick={() => acao(d, false)}><Icone nome="baixar" tamanho={14} /> Baixar</button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  )
}
