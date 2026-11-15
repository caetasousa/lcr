// Campo de partículas "dot matrix" em WebGL (Three.js + ShaderMaterial).
// Carregado sob demanda (import dinâmico) para não pesar o primeiro paint.
import * as THREE from 'three'

const vertex = /* glsl */ `
  uniform float uTempo;
  uniform float uDpr;
  uniform vec2  uPonteiro;
  attribute float aSemente;
  varying float vBrilho;
  varying float vProfundidade;

  void main() {
    vec3 p = position;
    float d = length(p.xy);

    // Respiração lenta: uma onda radial que sobe e desce o campo.
    float onda = sin(d * 0.55 - uTempo * 0.55) * 0.5 + 0.5;
    float respira = sin(uTempo * 0.35) * 0.5 + 0.5;
    p.z += onda * (0.6 + respira * 0.5);

    // O ponteiro "levanta" suavemente os pontos próximos.
    float perto = 1.0 - smoothstep(0.0, 6.0, distance(p.xy, uPonteiro));
    p.z += perto * 0.9;

    vec4 mv = modelViewMatrix * vec4(p, 1.0);
    gl_Position = projectionMatrix * mv;

    vProfundidade = clamp((-mv.z - 6.0) / 30.0, 0.0, 1.0);
    vBrilho = 0.35 + onda * 0.45 + perto * 0.6 + aSemente * 0.15;
    gl_PointSize = (2.2 + onda * 1.6 + perto * 2.5) * uDpr * (14.0 / -mv.z);
  }
`

const fragment = /* glsl */ `
  uniform vec3 uLime;
  uniform vec3 uVerde;
  varying float vBrilho;
  varying float vProfundidade;

  void main() {
    // Ponto redondo com borda suave.
    vec2 c = gl_PointCoord - 0.5;
    float r = length(c);
    float alfa = smoothstep(0.5, 0.15, r);
    // Gradiente: verde perto, lime ao fundo; some com a profundidade.
    vec3 cor = mix(uVerde, uLime, vProfundidade);
    alfa *= vBrilho * (1.0 - vProfundidade * 0.85);
    if (alfa < 0.01) discard;
    gl_FragColor = vec4(cor, alfa);
  }
`

export interface Cena {
  destruir: () => void
}

export function criarCena(canvas: HTMLCanvasElement, opts: { reduzirMovimento: boolean }): Cena {
  const renderer = new THREE.WebGLRenderer({ canvas, alpha: true, antialias: true, powerPreference: 'low-power' })
  const dpr = Math.min(window.devicePixelRatio || 1, 2) // DPR clamp
  renderer.setPixelRatio(dpr)
  renderer.setClearColor(0x000000, 0)

  const cena = new THREE.Scene()
  const camera = new THREE.PerspectiveCamera(60, 1, 0.1, 100)
  camera.position.set(0, -7, 9)
  camera.lookAt(0, 2, 0)

  // Grade esparsa de pontos (matriz), levemente desalinhada para não
  // parecer uma textura.
  const colunas = 90
  const linhas = 56
  const espaco = 0.62
  const pos = new Float32Array(colunas * linhas * 3)
  const semente = new Float32Array(colunas * linhas)
  let i = 0
  for (let y = 0; y < linhas; y++) {
    for (let x = 0; x < colunas; x++) {
      pos[i * 3] = (x - colunas / 2) * espaco
      pos[i * 3 + 1] = (y - linhas / 4) * espaco
      pos[i * 3 + 2] = 0
      semente[i] = Math.random()
      i++
    }
  }
  const geo = new THREE.BufferGeometry()
  geo.setAttribute('position', new THREE.BufferAttribute(pos, 3))
  geo.setAttribute('aSemente', new THREE.BufferAttribute(semente, 1))

  const uniforms = {
    uTempo: { value: 0 },
    uDpr: { value: dpr },
    uPonteiro: { value: new THREE.Vector2(999, 999) },
    uLime: { value: new THREE.Color('#C4F042') },
    uVerde: { value: new THREE.Color('#4EF746') },
  }
  const mat = new THREE.ShaderMaterial({
    vertexShader: vertex,
    fragmentShader: fragment,
    uniforms,
    transparent: true,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
  })
  const pontos = new THREE.Points(geo, mat)
  pontos.rotation.x = -0.35
  cena.add(pontos)

  // Ponteiro: deriva sutil da câmera (parallax) + "levantar" pontos.
  const alvo = new THREE.Vector2(0, 0)
  const atual = new THREE.Vector2(0, 0)
  const raio = new THREE.Raycaster()
  const plano = new THREE.Plane(new THREE.Vector3(0, 0, 1), 0)
  const ponto3 = new THREE.Vector3()
  const ndc = new THREE.Vector2()
  const aoMover = (e: PointerEvent) => {
    alvo.set((e.clientX / innerWidth) * 2 - 1, -(e.clientY / innerHeight) * 2 + 1)
  }
  addEventListener('pointermove', aoMover, { passive: true })

  const redimensiona = () => {
    const w = canvas.clientWidth
    const h = canvas.clientHeight
    renderer.setSize(w, h, false)
    camera.aspect = w / h
    camera.updateProjectionMatrix()
  }
  const ro = new ResizeObserver(redimensiona)
  ro.observe(canvas)
  redimensiona()

  const relogio = new THREE.Timer()
  let quadro = 0
  const desenha = () => {
    relogio.update()
    uniforms.uTempo.value = opts.reduzirMovimento ? 4 : relogio.getElapsed()
    atual.lerp(alvo, 0.04)
    camera.position.x = atual.x * 0.8
    camera.position.y = -7 + atual.y * 0.5
    camera.lookAt(0, 2, 0)

    // Converte o ponteiro para o plano dos pontos (espaço local).
    ndc.copy(atual)
    raio.setFromCamera(ndc, camera)
    pontos.updateMatrixWorld()
    const local = plano.clone().applyMatrix4(pontos.matrixWorld)
    if (raio.ray.intersectPlane(local, ponto3)) {
      pontos.worldToLocal(ponto3)
      uniforms.uPonteiro.value.set(ponto3.x, ponto3.y)
    }
    renderer.render(cena, camera)
  }

  const loop = () => {
    desenha()
    quadro = requestAnimationFrame(loop)
  }
  const visibilidade = () => {
    cancelAnimationFrame(quadro)
    if (!document.hidden && !opts.reduzirMovimento) loop()
  }
  document.addEventListener('visibilitychange', visibilidade)
  if (opts.reduzirMovimento) desenha() // "poster frame": um quadro estático
  else loop()

  return {
    destruir() {
      cancelAnimationFrame(quadro)
      removeEventListener('pointermove', aoMover)
      document.removeEventListener('visibilitychange', visibilidade)
      ro.disconnect()
      geo.dispose()
      mat.dispose()
      renderer.dispose()
    },
  }
}
