import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import * as pdfjs from 'pdfjs-dist'
import type { PDFDocumentProxy, PageViewport, RenderTask } from 'pdfjs-dist'
import workerUrl from 'pdfjs-dist/build/pdf.worker.min.mjs?url'
import type { Posicao } from '../api'
import Icone from './Icone'

pdfjs.GlobalWorkerOptions.workerSrc = workerUrl

// Caixa da assinatura em "unidades de página" (viewport com escala 1, origem
// no canto superior esquerdo, como na tela). Na hora de enviar, convertemos
// para pontos PDF (origem no canto inferior esquerdo) com convertToPdfPoint.
interface Caixa {
  pagina: number
  x: number
  y: number
  w: number
  h: number
}

const PADRAO = { w: 250, h: 66 }
const MIN = { w: 120, h: 34 }

type Arrasto = { modo: 'mover'; dx: number; dy: number } | { modo: 'redimensionar'; x0: number; y0: number; w0: number; h0: number }

interface Props {
  arquivo: File
  nome: string
  cpf?: string
  onPosicao: (p: Posicao | null) => void
  onErro: (msg: string) => void
}

export default function PosicionadorPdf({ arquivo, nome, cpf, onPosicao, onErro }: Props) {
  const [pdf, setPdf] = useState<PDFDocumentProxy | null>(null)
  const [pagina, setPagina] = useState(1)
  const [vp1, setVp1] = useState<PageViewport | null>(null)
  const [largura, setLargura] = useState(800)
  const [caixa, setCaixa] = useState<Caixa | null>(null)
  const canvas = useRef<HTMLCanvasElement>(null)
  const area = useRef<HTMLDivElement>(null)
  const sobreposicao = useRef<HTMLDivElement>(null)
  const arrasto = useRef<Arrasto | null>(null)

  // 1) Carrega o PDF no navegador (o arquivo ainda não foi enviado ao servidor).
  useEffect(() => {
    let cancelado = false
    let tarefa: ReturnType<typeof pdfjs.getDocument> | null = null
    ;(async () => {
      try {
        const dados = new Uint8Array(await arquivo.arrayBuffer())
        if (cancelado) return
        tarefa = pdfjs.getDocument({ data: dados })
        const doc = await tarefa.promise
        if (cancelado) return
        setPdf(doc)
        setPagina(doc.numPages) // como no gov.br, começa pela última página
        setCaixa(null)
      } catch {
        if (!cancelado) onErro('Não foi possível abrir este PDF.')
      }
    })()
    return () => {
      cancelado = true
      setPdf(null)
      tarefa?.destroy()
    }
  }, [arquivo, onErro])

  // Largura disponível (responsivo).
  useLayoutEffect(() => {
    const el = area.current
    if (!el) return
    const ro = new ResizeObserver(([e]) => setLargura(e.contentRect.width))
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  // Altura útil da tela: a página inteira deve caber, como no gov.br.
  const [alturaMax, setAlturaMax] = useState(() => Math.max(420, window.innerHeight - 260))
  useEffect(() => {
    const f = () => setAlturaMax(Math.max(420, window.innerHeight - 260))
    addEventListener('resize', f)
    return () => removeEventListener('resize', f)
  }, [])

  const escalaPara = (v: PageViewport) => Math.min(1.6, (largura - 36) / v.width, alturaMax / v.height)
  const escala = vp1 ? escalaPara(vp1) : 1

  // 2) Renderiza a página atual em um <canvas> (nítido em telas HiDPI).
  useEffect(() => {
    if (!pdf) return
    let tarefa: RenderTask | null = null
    let cancelado = false
    ;(async () => {
      const pg = await pdf.getPage(pagina)
      if (cancelado) return
      const v1 = pg.getViewport({ scale: 1 })
      setVp1(v1)
      const vp = pg.getViewport({ scale: escalaPara(v1) })
      const dpr = window.devicePixelRatio || 1
      const c = canvas.current!
      c.width = Math.floor(vp.width * dpr)
      c.height = Math.floor(vp.height * dpr)
      c.style.width = `${vp.width}px`
      c.style.height = `${vp.height}px`
      tarefa = pg.render({ canvas: c, viewport: vp, transform: dpr !== 1 ? [dpr, 0, 0, dpr, 0, 0] : undefined })
      await tarefa.promise.catch(() => {})
    })()
    return () => {
      cancelado = true
      tarefa?.cancel()
    }
  }, [pdf, pagina, largura, alturaMax])

  // 3) Converte a caixa para coordenadas PDF sempre que ela muda.
  useEffect(() => {
    if (!caixa || !vp1 || caixa.pagina !== pagina) {
      if (!caixa) onPosicao(null)
      return
    }
    const [ax, ay] = vp1.convertToPdfPoint(caixa.x, caixa.y)
    const [bx, by] = vp1.convertToPdfPoint(caixa.x + caixa.w, caixa.y + caixa.h)
    onPosicao({
      pagina: caixa.pagina,
      x1: Math.min(ax, bx), y1: Math.min(ay, by),
      x2: Math.max(ax, bx), y2: Math.max(ay, by),
    })
  }, [caixa, vp1, pagina, onPosicao])

  const limita = (c: Caixa): Caixa => {
    if (!vp1) return c
    const w = Math.max(MIN.w, Math.min(c.w, vp1.width))
    const h = Math.max(MIN.h, Math.min(c.h, vp1.height))
    return {
      ...c, w, h,
      x: Math.max(0, Math.min(c.x, vp1.width - w)),
      y: Math.max(0, Math.min(c.y, vp1.height - h)),
    }
  }

  const ponto = (e: React.PointerEvent) => {
    const r = sobreposicao.current!.getBoundingClientRect()
    return { px: (e.clientX - r.left) / escala, py: (e.clientY - r.top) / escala }
  }

  const aoPressionar = (e: React.PointerEvent) => {
    if (e.button !== 0) return
    const { px, py } = ponto(e)
    const alvo = (e.target as HTMLElement).closest<HTMLElement>('[data-alvo]')?.dataset.alvo
    const atual = caixa && caixa.pagina === pagina ? caixa : null
    if (alvo === 'alca' && atual) {
      arrasto.current = { modo: 'redimensionar', x0: px, y0: py, w0: atual.w, h0: atual.h }
    } else if (alvo === 'caixa' && atual) {
      arrasto.current = { modo: 'mover', dx: px - atual.x, dy: py - atual.y }
    } else {
      const w = atual?.w ?? PADRAO.w
      const h = atual?.h ?? PADRAO.h
      setCaixa(limita({ pagina, x: px - w / 2, y: py - h / 2, w, h }))
      arrasto.current = { modo: 'mover', dx: w / 2, dy: h / 2 }
    }
    sobreposicao.current!.setPointerCapture(e.pointerId)
  }

  const aoMover = (e: React.PointerEvent) => {
    const a = arrasto.current
    if (!a) return
    const { px, py } = ponto(e)
    setCaixa((c) => {
      if (!c) return c
      if (a.modo === 'mover') return limita({ ...c, x: px - a.dx, y: py - a.dy })
      return limita({ ...c, w: a.w0 + (px - a.x0), h: a.h0 + (py - a.y0) })
    })
  }

  const aoSoltar = () => {
    arrasto.current = null
  }

  const noRodape = () => {
    if (!vp1) return
    setCaixa(limita({ pagina, x: 36, y: vp1.height - PADRAO.h - 30, w: PADRAO.w, h: PADRAO.h }))
  }

  const n = pdf?.numPages ?? 0
  const caixaVisivel = caixa && caixa.pagina === pagina ? caixa : null
  const fs = caixaVisivel ? (caixaVisivel.h * escala) / 5.4 : 10

  return (
    <div className="posicionador">
      <div className="barra-paginas">
        <button disabled={pagina <= 1} onClick={() => setPagina((p) => p - 1)} aria-label="Página anterior">
          <Icone nome="esquerda" tamanho={16} />
        </button>
        <span className="pag">
          Página
          <select value={pagina} onChange={(e) => setPagina(Number(e.target.value))}>
            {Array.from({ length: n }, (_, i) => (
              <option key={i + 1} value={i + 1}>{i + 1}</option>
            ))}
          </select>
          de {n}
        </span>
        <button disabled={pagina >= n} onClick={() => setPagina((p) => p + 1)} aria-label="Próxima página">
          <Icone nome="direita" tamanho={16} />
        </button>
        <span className="espaco" />
        {caixa && <button className="link" onClick={() => setCaixa(null)}>Remover</button>}
        <button onClick={noRodape}><Icone nome="alvo" tamanho={16} /> Posicionar no rodapé</button>
      </div>

      {caixa && caixa.pagina !== pagina && (
        <div className="nota aviso">
          <Icone nome="alerta" />
          <span>
            A assinatura está na página {caixa.pagina}.{' '}
            <button className="link destaque" onClick={() => setPagina(caixa.pagina)}>Ir para ela</button>
          </span>
        </div>
      )}

      <div className="area-pdf" ref={area}>
        <div className="pagina-pdf" style={{ width: vp1 ? vp1.width * escala : undefined }}>
          <canvas ref={canvas} />
          <div
            ref={sobreposicao}
            className="sobreposicao"
            onPointerDown={aoPressionar}
            onPointerMove={aoMover}
            onPointerUp={aoSoltar}
            onPointerCancel={aoSoltar}
          >
            {!caixa && vp1 && (
              <div className="marcador-alvo">
                <div><Icone nome="alvo" tamanho={14} /> Clique onde a assinatura deve aparecer</div>
              </div>
            )}
            {caixaVisivel && (
              <div
                className="caixa-assinatura"
                data-alvo="caixa"
                style={{
                  left: caixaVisivel.x * escala,
                  top: caixaVisivel.y * escala,
                  width: caixaVisivel.w * escala,
                  height: caixaVisivel.h * escala,
                  fontSize: fs,
                }}
              >
                <div className="previa">
                  <div className="corpo">
                    <svg className="selo-previa" viewBox="0 0 100 100" aria-hidden="true">
                      <circle cx="50" cy="50" r="50" fill="#0c0c0c" />
                      <circle cx="50" cy="50" r="40" fill="none" stroke="#c4f042" strokeWidth="6" />
                      <polyline points="32,51 45,63 69,36" fill="none" stroke="#fff" strokeWidth="8" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                    <div className="textos">
                      <span className="r">DOCUMENTO ASSINADO DIGITALMENTE</span>
                      <b>{nome.toUpperCase()}</b>
                      {cpf && <span>CPF ***.{cpf.slice(4, 11)}-**</span>}
                      <span>data e hora da assinatura</span>
                    </div>
                  </div>
                  <div className="rodape-previa">Verifique a autenticidade em&nbsp;<b>{location.host}/validar</b></div>
                </div>
                <div className="alca" data-alvo="alca" title="Arraste para redimensionar" />
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
