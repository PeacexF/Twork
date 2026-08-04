<div align="center">

<picture>
  <img src=".github/img/twork.png" width="300" alt="twork Logo">
</picture>

# Twork

**Telegram Job hunting tool**

[![Go](https://img.shields.io/badge/Go-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![SQLite](https://img.shields.io/badge/SQLite-003B57?logo=sqlite&logoColor=white)](https://sqlite.org/)
[![Telegram](https://img.shields.io/badge/Telegram-26A5E4?logo=telegram&logoColor=white)](https://telegram.org/)
[![Docker](https://img.shields.io/badge/Docker-2496ED?logo=docker&logoColor=white)](https://www.docker.com/)

![License](https://img.shields.io/badge/License-MIT-black)
![Status](https://img.shields.io/badge/Status-Development-black)
![Search](https://img.shields.io/badge/Search-Keyword--Based-black)


</div>

A small tool I'm building to make job hunting on Telegram significantly less painful.

The idea is simple: instead of checking dozens (or hundreds) of channels every day, Twork monitors them for you, indexes every message locally, and lets you search through everything almost instantly.

No AI(i have bad experience with the api honestly), no cloud services, no external APIs.

Just deterministic keyword matching, a local database, a Telegram bot interface, and an optional web dashboard.

## Planned Features

* Monitor Telegram channels and groups
* Local message indexing
* Fast keyword search
* Positive and negative keyword filters
* Instant notifications for matching posts
* Bookmarks and saved jobs
* Simple statistics dashboard
* Direct links to the original Telegram messages
* SQLite-based storage (PostgreSQL may come later)
* Resume broadcasting into monitored groups (see below)
* Optional web dashboard for managing chats and the resume

## Resume broadcasting

Most "vacancies" posted in Telegram job channels are stale or outright fake. What actually
works is having your own pitch visible in the groups people who need help are already in --
they see it and DM *you*, rather than the other way around.

Twork can periodically re-post your resume/pitch into the **groups** you monitor, on a
schedule you set, per chat. A few things are deliberate, not incidental:

* **Off by default**, per chat -- you turn it on from the bot or the web dashboard.
* **Groups only.** Channels are broadcast-only (a regular member usually can't post there,
  and an admin "sending" would blast every subscriber instead of building presence), so
  broadcasting can't be enabled on one. Enforced in the storage layer, not just the UI.
* **DMs are never touched, in either direction.** No detection of inbound messages, no
  auto-reply. What happens after someone reaches out to you is entirely on you.
* **Hardcoded, overridable safety limits**: a minimum delay between two sends into the same
  group, and a maximum number of sends per hour across every group combined. Both exist to
  keep the account from getting rate-limited or banned for spam-like behavior -- lowering
  them is strongly discouraged, even on a Telegram Premium account. See `compliance:` in
  `config.example.yaml`.
* Only works with the `mtproto` source -- `rsshub` is read-only and can't send anything.

## What Twork is **not**

* An AI job assistant
* A job board
* A resume generator (you write the pitch; Twork only sends it, on your explicit say-so)

The goal is to stay lightweight, predictable, and completely self-hosted.

## Why?

Telegram has become one of the biggest places to find developer jobs, freelance gigs, and startup opportunities, but its search is limited and keeping up with hundreds of channels quickly becomes impossible.

Twork is an attempt to fix that while staying as simple as possible.

## Status

Early development.

The architecture and feature set are still changing, so expect breaking changes while the project takes shape.

## Contributing

Ideas, bug reports, and pull requests are always welcome.
