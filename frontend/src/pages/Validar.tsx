import { useEffect, useState } from 'react'
import { api, dataHora, type Assinatura, type ResultadoValidacao } from '../api'
import SeletorArquivo from '../components/SeletorArquivo'
import Icone from '../components/Icone'

interface Props {
  inicial: { blob: Blob; nome: string } | null
  onConsumido: () => void
}

const traduzAviso = (a: string) =>
  a.includes('signature time as fallback')
    ? 'A data foi declarada na própria assinatura: não há carimbo de tempo de uma ACT.'
    : a

function Checagem({ estado, titulo, children }: { estado: 'ok' | 'falha' | 'alerta'; titulo: string; children: React.ReactNode }) {
  return (
    <li className={estado}>
      <span className="ic"><Icone nome={estado === 'ok' ? 'check' : estado === 'falha' ? 'x' : 'alerta'} tamanho={14} /></span>
      <div>
        <b>{titulo}</b>
        <p>{children}</p>
      </div>
    </li>
  )
}

function CartaoAssinatura({ a, n }: { a: Assinatura; n: number }) {
  const revogada = a.lcr.revogado_em != null
  const antes = a.lcr.revogado_antes_da_assinatura
  return (
    <article className="painel assinatura revelar" style={{ '--i': n } as React.CSSProperties}>
      <div className="assinatura-topo">
        <span className="chip">#{n}</span>
        <h3 className="subtitulo">{a.nome || a.certificado?.titular || 'Signatário desconhecido'}</h3>
        <span className={'chip ' + (a.valida ? 'ok' : 'falha')}><span className="ponto" />{a.valida ? 'Aprovada' : 'Reprovada'}</span>
      </div>
      <div className="assinatura-corpo">
        <div className="meta">
          <span>Assinado em <b>{dataHora(a.data_assinatura)}</b></span>
          {a.motivo && <span>Motivo <b>{a.motivo}</b></span>}
          {a.contato && <span>{a.contato}</span>}
        </div>
        <ul className="checagens">
          <Checagem estado={a.integra ? 'ok' : 'falha'} titulo={a.integra ? 'Documento íntegro' : 'Documento alterado após a assinatura'}>
            {a.integra
              ? 'O SHA-256 recalculado confere com o hash assinado, e a assinatura RSA confere com a chave pública do certificado.'
              : 'O hash do conteúdo não confere com o que foi assinado.'}
          </Checagem>
          <Checagem estado={a.cadeia_confiavel ? 'ok' : 'falha'} titulo={a.cadeia_confiavel ? 'Cadeia de certificação confiável' : 'Cadeia de certificação não confiável'}>
            {a.cadeia_confiavel
              ? 'Usuário → AC ACME Assinatura Digital → AC ACME Raiz G2.'
              : 'O certificado não foi emitido por uma AC em que este validador confia.'}
          </Checagem>
          <Checagem
            estado={antes || a.revogado_na_lcr ? 'falha' : revogada ? 'alerta' : 'ok'}
            titulo={!revogada ? 'Certificado não revogado' : antes ? 'Certificado já estava revogado ao assinar' : 'Certificado revogado depois da assinatura'}
          >
            {revogada
              ? `LCR nº ${a.lcr.numero_lcr}: revogado em ${dataHora(a.lcr.revogado_em)} (${a.lcr.motivo}).` +
                (antes ? '' : ' A assinatura é anterior e continua válida.')
              : `Consultada a LCR nº ${a.lcr.numero_lcr}, assinada pela AC ACME Assinatura Digital.`}
          </Checagem>
        </ul>

        {a.erros.length > 0 && (
          <div className="erros-tecnicos">{a.erros.map((e, i) => <pre key={i}>{e}</pre>)}</div>
        )}
        {a.avisos.length > 0 && (
          <ul className="avisos">
            {a.avisos.map((w, i) => <li key={i}><Icone nome="alerta" tamanho={14} />{traduzAviso(w)}</li>)}
          </ul>
        )}

        {a.cadeia.length > 0 && (
          <details>
            <summary>Cadeia de certificados ({a.cadeia.length})</summary>
            <ul className="arvore">
              {[...a.cadeia].reverse().map((c) => (
                <li key={c.serial}>
                  <div className="no">
                    <b>{c.titular}</b>
                    <span className="mono">série {c.serial} · até {new Date(c.valido_ate).toLocaleDateString('pt-BR')}</span>
                  </div>
                </li>
              ))}
            </ul>
          </details>
        )}
      </div>
    </article>
  )
}

export default function Validar({ inicial, onConsumido }: Props) {
  const [nome, setNome] = useState('')
  const [res, setRes] = useState<ResultadoValidacao | null>(null)
  const [erro, setErro] = useState('')
  const [ocupado, setOcupado] = useState(false)

  const valida = async (blob: Blob, n: string) => {
    setNome(n)
    setRes(null)
    setErro('')
    setOcupado(true)
    try {
      setRes(await api.validar(blob, n))
    } catch (e) {
      setErro((e as Error).message)
    } finally {
      setOcupado(false)
    }
  }

  useEffect(() => {
    if (inicial) {
      valida(inicial.blob, inicial.nome)
      onConsumido()
    }
  }, [inicial]) // eslint-disable-line

  return (
    <div>
      <header className="cabecalho-pagina revelar">
        <span className="rotulo">Verificação pública</span>
        <h1 className="titulo">Validar assinatura</h1>
        <p>Confere a integridade do PDF, a cadeia de certificação até a raiz e a situação do certificado na LCR. Não precisa de login.</p>
      </header>

      <div className="painel revelar" style={{ '--i': 1 } as React.CSSProperties}>
        <SeletorArquivo texto={nome ? 'Validar outro documento' : 'Escolha o PDF assinado'} onArquivo={(f) => valida(f, f.name)} />
      </div>

      {ocupado && <div className="carregando"><span className="giro" /> Verificando assinaturas…</div>}
      {erro && <p className="erro">{erro}</p>}

      {res && (
        <section>
          <div className={'veredito revelar ' + (res.valido ? 'ok' : 'falha')}>
            <span className="icone-v"><Icone nome={res.valido ? 'check' : 'x'} tamanho={24} /></span>
            <div>
              <h2 className="subtitulo">
                {res.assinaturas.length === 0 ? 'Nenhuma assinatura encontrada' : res.valido ? 'Todas as assinaturas são válidas' : 'Há assinaturas inválidas'}
              </h2>
              <p>{nome} · {res.paginas} página(s) · {res.assinaturas.length} assinatura(s)</p>
            </div>
          </div>
          <div className="hash-arquivo mono">SHA-256 {res.sha256}</div>
          {res.erro && res.assinaturas.length === 0 && <p className="erro">{res.erro}</p>}
          {res.assinaturas.map((a, i) => <CartaoAssinatura key={i} a={a} n={i + 1} />)}
        </section>
      )}
    </div>
  )
}
