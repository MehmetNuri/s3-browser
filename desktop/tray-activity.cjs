const { nativeImage } = require('electron')

// Direction of the moving arrow per operation; other kinds use an orbiting dot.
const arrows = { upload: [0, -1], download: [0, 1], copy: [1, 0], move: [1, 0] }
// Brand orange while working, then green or red for the outcome (BGR).
const colors = { active: [0x20, 0x82, 0xf5], done: [0x3d, 0x80, 0x15], failed: [0x1c, 0x1c, 0xb9] }

// Draws a full-size status icon: a small corner badge is unreadable at tray
// size, so the whole icon becomes one large glyph on a rounded square.
// Template (macOS) icons carry only alpha, so only the glyph is drawn.
function drawFrame(base, glyph, step, steps, template) {
  const scaleFactor = template ? 2 : 1
  const { width, height } = base.getSize(scaleFactor)
  const pixels = Buffer.alloc(width * height * 4)
  const put = (x, y, [b, g, r]) => {
    if (x < 0 || y < 0 || x >= width || y >= height) return
    const i = (y * width + x) * 4
    pixels[i] = b; pixels[i + 1] = g; pixels[i + 2] = r; pixels[i + 3] = 255
  }
  if (!template) {
    const fill = glyph === 'done' ? colors.done : glyph === 'failed' ? colors.failed : colors.active
    const corner = Math.round(width * 0.22)
    for (let y = 0; y < height; y++) for (let x = 0; x < width; x++) {
      // Distance into the nearest corner square decides whether the pixel is rounded off.
      const dx = Math.max(corner - x, x - (width - 1 - corner), 0), dy = Math.max(corner - y, y - (height - 1 - corner), 0)
      if (dx * dx + dy * dy <= corner * corner) put(x, y, fill)
    }
  }
  const ink = template ? [0, 0, 0] : [255, 255, 255]
  const thickness = Math.max(2, Math.round(width / 9))
  const margin = Math.round(width * 0.12)
  const dot = (x, y, size = thickness) => {
    for (let dy = 0; dy < size; dy++) for (let dx = 0; dx < size; dx++) {
      const px = Math.round(x - size / 2 + dx), py = Math.round(y - size / 2 + dy)
      if (px >= margin && py >= margin && px < width - margin && py < height - margin) put(px, py, ink)
    }
  }
  const line = (x1, y1, x2, y2) => {
    const count = Math.ceil(Math.max(Math.abs(x2 - x1), Math.abs(y2 - y1)) * 2) || 1
    for (let i = 0; i <= count; i++) dot(x1 + ((x2 - x1) * i) / count, y1 + ((y2 - y1) * i) / count)
  }
  const cx = width / 2, cy = height / 2, size = width * 0.27
  if (glyph === 'done') {
    line(cx - size, cy + size * 0.1, cx - size * 0.3, cy + size * 0.8); line(cx - size * 0.3, cy + size * 0.8, cx + size, cy - size * 0.7)
  } else if (glyph === 'failed') {
    line(cx, cy - size, cx, cy + size * 0.2); dot(cx, cy + size, thickness + 1)
  } else if (arrows[glyph]) {
    // The arrow travels in its direction and starts over from behind.
    const [dx, dy] = arrows[glyph]
    const shift = (step / (steps - 1) - 0.5) * width * 0.3
    const tipX = cx + dx * (size + shift), tipY = cy + dy * (size + shift)
    line(tipX, tipY, tipX - dx * size * 2, tipY - dy * size * 2)
    line(tipX, tipY, tipX - dx * size - dy * size, tipY - dy * size - dx * size)
    line(tipX, tipY, tipX - dx * size + dy * size, tipY - dy * size + dx * size)
  } else {
    // Three dots chasing each other around the centre.
    for (const [offset, weight] of [[0, thickness + 2], [-1.5, thickness + 1], [-3, thickness]]) {
      const angle = ((step + offset) / steps) * Math.PI * 2
      dot(cx + Math.cos(angle) * size, cy + Math.sin(angle) * size, weight)
    }
  }
  return nativeImage.createFromBitmap(pixels, { width, height, scaleFactor })
}

// Shows what the application is doing on a tray icon: an animated badge
// while transfers run, then a short success or failure mark.
exports.createTrayActivity = (tray, image, { template = false } = {}) => {
  const base = !template && image.getSize().width > 64 ? image.resize({ width: 32, height: 32 }) : image
  const frames = new Map()
  let timer, step = 0, shown = 'idle'
  const frame = (glyph, index, steps) => {
    const key = `${glyph}:${index}`
    if (!frames.has(key)) {
      const drawn = drawFrame(base, glyph, index, steps, template)
      if (template) drawn.setTemplateImage(true)
      frames.set(key, drawn)
    }
    return frames.get(key)
  }
  const stop = () => { clearInterval(timer); timer = undefined }
  return {
    update(status) {
      if (tray.isDestroyed()) return stop()
      const glyph = status.state === 'active' ? (arrows[status.kind] ? status.kind : 'orbit') : status.state
      if (glyph === shown) return
      shown = glyph
      stop()
      if (glyph === 'idle') return tray.setImage(image)
      if (glyph === 'done' || glyph === 'failed') return tray.setImage(frame(glyph, 0, 1))
      const steps = arrows[glyph] ? 5 : 8
      step = 0
      const draw = () => { if (tray.isDestroyed()) return stop(); tray.setImage(frame(glyph, step++ % steps, steps)) }
      draw()
      timer = setInterval(draw, 220)
    },
    stop
  }
}
