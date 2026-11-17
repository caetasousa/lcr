import { lazy, Suspense, useCallback, useState } from 'react'
import { api, baixar, type Eu, type PdfAssinado, type Posicao } from '../api'
import SeletorArquivo from '../components/SeletorArquivo'
import Modal from '../components/Modal'
import Icone from '../components/Icone'
import { navegar } from '../App'

// O pdf.js é grande: só é baixado quando o usuário escolhe um arquivo.
const PosicionadorPdf = lazy(() => import('../components/PosicionadorPdf'))

interface Props {
  eu: Eu
  onValidar: (blob: Blob, nome: string) => void
}

function Etapas({ atual }: { atual: 1 | 2 | 3 }) {
  const nomes = ['Enviar documento', 'Posicionar assinatura', 'Assinar']
  return (
    <div className="etapas">
      {nomes.map((n, i) => (
        <div key={n} className={'etapa ' + (i + 1 === atual ? 'atual' : i + 1 < atual ? 'feita' : '')}>
          <span className="n">{i + 1 < atual ? <Icone nome="check" tamanho={12} /> : i + 1}</span>
          <span>{n}</span>
        </div>
      ))}
    </div>
  )
}

export default function Assinar({ eu, onValidar }: Props) {
  const [arquivo, setArquivo] = useState<File | null>(null)
  const [posicao, setPosicao] = useState<Posicao | null>(null)
  const [confirmar, setConfirmar] = useState(false)
  const [senha, setSenha] = useState('')
  const [motivo, setMotivo] = useState('')
  const [erro, setErro] = useState('')
  const [ocupado, setOcupado] = useState(false)
  const [resultado, setResultado] = useState<PdfAssinado | null>(null)

  const onErro = useCallback((m: string) => setErro(m), [])
  const fechar = useCallback(() => !ocupado && setConfirmar(false), [ocupado])

  const reiniciar = () => {
    setArquivo(null)
    setPosicao(null)
    setResultado(null)
    setErro('')
  }

  const assinar = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!arquivo || !posicao) return
    setOcupado(true)
    setErro('')
    try {
      setResultado(await api.assinar(arquivo, posicao, senha, motivo))
      setConfirmar(false)
      setSenha('')
    } catch (err) {
      setErro((err as Error).message)
    } finally {
      setOcupado(false)
    }
  }

  if (eu.certificado.situacao !== 'válido') {
    return (
      <div className="painel sucesso revelar">
        <div className="selo" style={{ background: 'var(--perigo)', boxShadow: '0 0 32px rgb(255 95 87 / .45)' }}>
          <Icone nome="alerta" tamanho={28} />
        </div>
        <h2 className="titulo">Certificado {eu.certificado.situacao}</h2>
        <p className="sutil" style={{ marginTop: 12 }}>Para voltar a assinar, emita um novo certificado.</p>
        <button className="primario grande" onClick={() => navegar('/certificado')}>Ir para Meu certificado</button>
      </div>
    )
  }

  if (resultado) {
    return (
      <div className="painel sucesso revelar">
        <div className="selo"><Icone nome="check" tamanho={30} /></div>
        <span className="rotulo">PAdES · CMS · SHA-256 + RSA</span>
        <h2 className="titulo" style={{ marginTop: 8 }}>Documento assinado</h2>
        <p className="sutil" style={{ marginTop: 12 }}>
          <b style={{ color: 'var(--texto)' }}>{resultado.nome}</b> foi assinado com o certificado de {eu.nome}.
        </p>
        <div className="hashes">
          <div className="hash"><span className="rotulo">SHA-256 original</span><code>{resultado.sha256Original}</code></div>
          <div className="hash"><span className="rotulo">SHA-256 assinado</span><code>{resultado.sha256Assinado}</code></div>
        </div>
        <p className="fraco" style={{ fontSize: 12.5 }}>
          Os hashes diferem porque a assinatura foi anexada ao PDF (atualização incremental). Os bytes originais continuam
          intactos dentro do arquivo assinado.
        </p>
        <div className="acoes centro" style={{ marginTop: 'var(--e4)' }}>
          <button className="primario grande" onClick={() => baixar(resultado.blob, resultado.nome)}>
            <Icone nome="baixar" tamanho={16} /> Baixar PDF assinado
          </button>
          <button className="grande" onClick={() => window.open(URL.createObjectURL(resultado.blob), '_blank')}>
            <Icone nome="olho" tamanho={16} /> Abrir
          </button>
          <button className="grande" onClick={() => onValidar(resultado.blob, resultado.nome)}>
            <Icone nome="escudo" tamanho={16} /> Validar
          </button>
        </div>
        <button className="link" style={{ marginTop: 'var(--e3)' }} onClick={reiniciar}>Assinar outro documento</button>
      </div>
    )
  }

  return (
    <div>
      <header className="cabecalho-pagina revelar">
        <span className="rotulo">Assinatura eletrônica</span>
        <h1 className="titulo">Assinar documento</h1>
      </header>
      <Etapas atual={!arquivo ? 1 : !posicao ? 2 : 3} />

      {!arquivo ? (
        <div className="painel revelar" style={{ '--i': 1 } as React.CSSProperties}>
          <SeletorArquivo texto="Escolha o PDF que você quer assinar" onArquivo={(f) => { setErro(''); setArquivo(f) }} />
          <p className="fraco" style={{ fontSize: 13, marginTop: 'var(--e2)' }}>
            Se outra pessoa já assinou, envie o PDF <b>já assinado</b> por ela, não o original: só assim as duas assinaturas ficam no mesmo arquivo.
          </p>
          {erro && <p className="erro">{erro}</p>}
        </div>
      ) : (
        <div className="painel revelar">
          <div className="painel-cabecalho" style={{ marginBottom: 0 }}>
            <div className="doc-cabecalho">
              <div className="doc-icone"><Icone nome="documento" /></div>
              <div style={{ minWidth: 0 }}>
                <div className="nome">{arquivo.name}</div>
                <div className="fraco" style={{ fontSize: 12 }}>{(arquivo.size / 1024).toFixed(0)} KB · arraste o quadro para mover e o canto para redimensionar</div>
              </div>
            </div>
            <button onClick={reiniciar}>Trocar arquivo</button>
          </div>

          <Suspense fallback={<div className="carregando"><span className="giro" /> Carregando visualizador…</div>}>
            <PosicionadorPdf arquivo={arquivo} nome={eu.nome} cpf={eu.cpf} onPosicao={setPosicao} onErro={onErro} />
          </Suspense>
          {erro && <p className="erro">{erro}</p>}

          <div className="rodape-acao">
            <span className="coords">
              {posicao
                ? `pág ${posicao.pagina} · x ${posicao.x1.toFixed(0)}→${posicao.x2.toFixed(0)} pt · y ${posicao.y1.toFixed(0)}→${posicao.y2.toFixed(0)} pt`
                : 'aguardando a posição da assinatura'}
            </span>
            <button className="primario grande" disabled={!posicao} onClick={() => { setErro(''); setConfirmar(true) }}>
              <Icone nome="caneta" tamanho={16} /> Assinar documento
            </button>
          </div>
        </div>
      )}

      {confirmar && (
        <Modal titulo="Confirmar assinatura" onFechar={fechar}>
          <form onSubmit={assinar}>
            <p className="sutil">
              Você vai assinar <b style={{ color: 'var(--texto)' }}>{arquivo?.name}</b> como{' '}
              <b style={{ color: 'var(--texto)' }}>{eu.nome}</b>.
            </p>
            <label>
              <span>Motivo (opcional)</span>
              <input value={motivo} onChange={(e) => setMotivo(e.target.value)} placeholder="Ex.: Aceite do contrato" maxLength={120} />
            </label>
            <label>
              <span>Sua senha</span>
              <input type="password" value={senha} onChange={(e) => setSenha(e.target.value)} autoFocus required autoComplete="current-password" />
            </label>
            <div className="nota">
              <Icone nome="chave" />
              <span>A senha libera a sua chave privada só durante esta assinatura.</span>
            </div>
            {erro && <p className="erro" role="alert">{erro}</p>}
            <div className="acoes direita" style={{ marginTop: 'var(--e3)' }}>
              <button type="button" onClick={() => setConfirmar(false)} disabled={ocupado}>Cancelar</button>
              <button className="primario" disabled={ocupado}>
                {ocupado ? <><span className="giro" /> Assinando…</> : 'Assinar'}
              </button>
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}
