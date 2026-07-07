import type { TailscalePeer } from "../../contracts/devices"

type PeerMatch = {
  index: number
  score: number
}

export function visibleTailscalePeers(peers: TailscalePeer[], query: string) {
  const normalizedQuery = normalizeSearchQuery(query)
  const matches: PeerMatch[] = []

  for (const [index, peer] of peers.entries()) {
    const target = normalizeSearchQuery(
      [peer.hostname, peer.dns_name, peer.ip, ...(peer.ips || []), peer.os, ...(peer.tags || [])]
        .filter(Boolean)
        .join(" "),
    )
    const score = fuzzyScore(normalizedQuery, target)
    if (score <= 0) {
      continue
    }
    matches.push({ index, score })
  }

  matches.sort((left, right) => {
    if (left.score !== right.score) {
      return right.score - left.score
    }

    const leftPeer = peers[left.index]
    const rightPeer = peers[right.index]
    if (leftPeer.recommended !== rightPeer.recommended && normalizedQuery === "") {
      return leftPeer.recommended ? -1 : 1
    }

    return left.index - right.index
  })

  return matches.map((match) => peers[match.index])
}

export function normalizeSearchQuery(value: string) {
  return value.toLowerCase().trim()
}

export function fuzzyScore(query: string, target: string) {
  if (query === "") {
    return 1
  }
  if (target === "") {
    return 0
  }
  if (target.includes(query)) {
    let score = 100 - (target.length - query.length)
    if (target.startsWith(query)) {
      score += 25
    }
    return score
  }

  let queryIndex = 0
  let run = 0
  let score = 0
  for (let index = 0; index < target.length && queryIndex < query.length; index += 1) {
    if (target[index] === query[queryIndex]) {
      queryIndex += 1
      run += 1
      score += 5 + run * 2
    } else {
      run = 0
    }
  }

  if (queryIndex !== query.length) {
    return 0
  }

  return score
}
