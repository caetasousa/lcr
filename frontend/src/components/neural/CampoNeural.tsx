import { useEffect, useRef, useState } from 'react'

function suportaWebGL() {
  try {
    const c = document.createElement('canvas')
    return !!(c.getContext('webgl2') || c.getContext('webgl'))
  } catch {
    return false
  }
}

/**
 * Fundo imersivo em tela cheia. Enquanto o Three.js carrega (ou se não
 * houver WebGL), o "DOM fallback" desenha a mesma matriz de pontos com CSS.
 */
export default function CampoNeural() {
  const canvas = useRef<HTMLCanvasElement>(null)
  const [pronto, setPronto] = useState(false)

  useEffect(() => {
    if (!suportaWebGL()) return
    let destruir: (() => void) | undefined
    let cancelado = false
    const reduzir = matchMedia('(prefers-reduced-motion: reduce)').matches
    import('./cena').then(({ criarCena }) => {
      if (cancelado || !canvas.current) return
      try {
        destruir = criarCena(canvas.current, { reduzirMovimento: reduzir }).destruir
        setPronto(true)
      } catch {
        /* sem WebGL utilizável: o fallback continua visível */
      }
    })
    return () => {
      cancelado = true
      destruir?.()
    }
  }, [])

  return (
    <div className="campo-neural" aria-hidden="true">
      <div className={'campo-fallback' + (pronto ? ' oculto' : '')} />
      <canvas ref={canvas} className={pronto ? 'visivel' : ''} />
      <div className="campo-vinheta" />
    </div>
  )
}
