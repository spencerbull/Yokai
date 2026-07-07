import { useEffect, useMemo, useState } from "react"

type MarqueeTextProps = {
  active?: boolean
  bg?: string
  fg: string
  text: string
  width: number
}

export function MarqueeText(props: MarqueeTextProps) {
  const [offset, setOffset] = useState(0)
  const normalizedWidth = Math.max(1, props.width)
  const shouldAnimate = props.active && props.text.length > normalizedWidth

  useEffect(() => {
    if (!shouldAnimate) {
      setOffset(0)
      return
    }

    const timer = setInterval(() => {
      setOffset((current) => current + 1)
    }, 180)

    return () => {
      clearInterval(timer)
    }
  }, [shouldAnimate])

  const rendered = useMemo(() => {
    if (props.text.length <= normalizedWidth) {
      return props.text.padEnd(normalizedWidth, " ")
    }

    if (!shouldAnimate) {
      return `${props.text.slice(0, Math.max(1, normalizedWidth - 1))}…`
    }

    const spacer = "   "
    const loop = `${props.text}${spacer}${props.text}${spacer}`
    const start = offset % (props.text.length + spacer.length)
    return loop.slice(start, start + normalizedWidth).padEnd(normalizedWidth, " ")
  }, [normalizedWidth, offset, props.text, shouldAnimate])

  return (
    <text fg={props.fg} bg={props.bg}>
      {rendered}
    </text>
  )
}
