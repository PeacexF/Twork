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

Just deterministic keyword matching, a local database, and a Telegram bot interface.

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

## What Twork is **not**

* An AI job assistant
* A job board
* An automatic application bot
* A resume generator

The goal is to stay lightweight, predictable, and completely self-hosted.

## Why?

Telegram has become one of the biggest places to find developer jobs, freelance gigs, and startup opportunities, but its search is limited and keeping up with hundreds of channels quickly becomes impossible.

Twork is an attempt to fix that while staying as simple as possible.

## Status

Early development.

The architecture and feature set are still changing, so expect breaking changes while the project takes shape.

## Contributing

Ideas, bug reports, and pull requests are always welcome.
