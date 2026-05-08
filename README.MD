# MasterDnsVPN Project 🔐

## | [نسخه فارسی](https://github.com/masterking32/MasterDnsVPN/blob/main/README_FA.MD) | [English Version](https://github.com/masterking32/MasterDnsVPN/blob/main/README.MD) |

**MasterDnsVPN** is a scientific and research-oriented project for carrying TCP traffic through DNS queries and responses. In broad goal, it is similar to projects such as DNSTT or SlipStream, but it follows a fundamentally different structure and implementation approach.
This system is designed around compatibility with many resolver behaviors and harsh network conditions, with the goal of preserving the highest possible stability and data delivery even in the worst cases.

> **Personal note:** I forked this for studying DNS tunneling techniques. The comparison table below is what initially caught my attention — the overhead difference vs DNSTT is significant.
>
> **Study notes:** The ~5–7B transport header overhead is achieved by stripping redundant framing and encoding only essential sequence/ack fields. Worth digging into the ARQ implementation to understand how retransmission is handled without inflating header size.
>
> **TODO:** Cross-reference the multipath resolver logic with the QUIC multipath spec (RFC 9000) to see where the design diverges — particularly around how packet loss on one resolver path triggers duplication vs. retransmission on alternates.

[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/masterking32/MasterDnsVPN) [![oosmetrics](https://api.oosmetrics.com/api/v1/badge/achievement/5c7b2ce0-0af6-4648-8ded-fd1e847096cd.svg)](https://oosmetrics.com/repo/masterking32/MasterDnsVPN)

### 📊 MasterDnsVPN Compared with Similar Projects

| Feature | SlipStream | DNSTT | MasterDnsVPN |
| :--- | :--- | :--- | :--- |
| Protocol type | Advanced DNS tunnel | Classic DNS tunnel | Advanced DNS tunnel / VPN |
| Transport protocol | QUIC | KCP + Noise | Custom protocol + ARQ |
| Transport header overhead | 🟠 ~24B | 🔴 ~59B | 🟢 ~5–7B<br>≈88% lower than DNSTT<br>≈71% lower than SlipStream |
| Encryption style | TLS 1.3 (inside QUIC) | Noise (Curve25519) | AES / ChaCha20 / XOR (if XOR is used: lightweight with lower security and no extra overhead) |
| Architecture | Unified (QUIC handles everything) | Multi-layered (KCP + SMUX + Noise) | 🟢 Lightweight custom design optimized for DNS |
| Speed | 🟡 High (up to ~5× faster than DNSTT) | 🔴 Medium | 🟢 Faster than others<br>Up to ~9× faster than DNSTT<br>Up to ~3.6× faster than SlipStream |
| Stability under packet loss | 🟡 Good | 🟠 Medium | 🟢 Very high (Multipath + ARQ) |
| Multi-resolver support | Yes (multipath) | ❌ | Yes — advanced (multi-resolver + duplication) |
| Resilience under heavy censorship | Good | Medium | Very strong (a core project goal) |
| Setup complexity | Medium | Simple | Easier installation<br>More complex only if you heavily customize advanced settings |
| SOCKS5 support | Yes | Yes | Optimized for SOCKS5 / SOCKS4 with reduced SOCKS overhead |
| Shadowsocks support | ✅ | ❌ | Indirectly: TCP Forwarding mode can carry TCP-based protocols<br>e.g. Shadowsocks, VLESS/VMess, etc. |
| Real multipath | Yes (QUIC multipath) | ❌ | Yes (multi-resolver + duplication) |
| Adaptive routing | Limited | ❌ | Advanc