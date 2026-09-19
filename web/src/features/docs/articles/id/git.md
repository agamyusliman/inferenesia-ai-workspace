---
title: Git
category: git
summary: Stage, commit, push, fetch, Actions, aturan workflow, secrets, dan bedanya dengan undo
keywords: git stage commit push pull fetch branch remote actions secrets undo
order: 50
---

# Git

Panel **Git** bekerja pada folder **workspace** aktif. Untuk version control harian: status, stage, commit, fetch, pull, push, plus intip GitHub Actions bila repo terhubung.

## Buka Git

1. Buka **workspace** folder (Sessions tidak punya strip Git).
2. Buka view **Git** dari activity rail / chrome layout.
3. Pastikan path cocok dengan repo yang kamu maksud (nested repo bisa tampil sebagai chip).

## Aksi sehari-hari

| Aksi | Fungsi |
|------|--------|
| **Status / diff** | Lihat file berubah + diff per baris |
| **Stage** | Masukkan file terpilih (atau hunk, bila ada) ke index |
| **Unstage** | Keluarkan dari index tanpa hapus edit di working tree |
| **Commit** | Buat commit dengan message di branch saat ini |
| **Fetch** | Update remote refs tanpa merge |
| **Pull** | Tarik commit remote ke branch (sesuai setup remote) |
| **Push** | Publikasikan commit lokal ke remote |

Loop tipikal:

1. Buat/terima perubahan (kamu atau agent).
2. Review diff.
3. Stage yang masuk commit ini.
4. Tulis message jelas (kenapa, bukan cuma apa).
5. Commit.
6. Push saat remote perlu update.

## Branch dan remote

- Cek nama **branch** di header Git sebelum commit.
- Prefer branch fitur untuk eksperimen agent yang ramai.
- **Fetch** dulu sebelum merge besar supaya tahu apa yang bergerak di remote.
- Push ditolak? Pull/rebase sesuai aturan tim, lalu push lagi. Inferenesia tidak “membuatkan” force-push untukmu.

## GitHub Actions

Kalau repo di GitHub dan credential mengizinkan, area **Actions** bisa menampilkan workflow run terbaru. Berguna untuk:

- Cek CI gagal setelah push
- Buka run gagal untuk log (browser / UI terhubung)
- Hindari tebak-tebakan “main masih hijau?”

Tidak semua setup privat menampilkan Actions; data kosong biasanya auth atau visibilitas remote, bukan status Git yang rusak.

## File aturan workflow

Banyak tim menyimpan aturan agent/kontributor di repo (`AGENTS.md`, `CONTRIBUTING.md`, dll.). Agent bisa membacanya saat workspace terbuka. Taruh aturan project yang tahan lama di situ:

- Cara jalankan test
- Naming branch
- “Jangan sentuh generated/”
- Ekspektasi review

Ini beda dari catatan Multi Brain pribadi di project (lihat [Tool agent](agent-tools)).

## Secrets dan keamanan

| Lakukan | Jangan |
|---------|--------|
| Jaga `.env` + key di luar commit | Commit API key, token, URL privat ber-credential |
| Andalkan `.gitignore` untuk env/config lokal | Force-add file secret yang di-ignore |
| Rotate key kalau sempat masuk history git | Paste secret production ke chat seenaknya |

Kalau agent stage file mencurigakan, **unstage** lalu perbaiki `.gitignore` sebelum commit.

## Git vs Undo / Revert agent

| Kebutuhan | Pakai |
|-----------|-------|
| Undo write agent terakhir, dirty WIP tetap | **Undo** atau **Revert agent** |
| Checkpoint yang bisa push / PR | **Commit** |
| Buang perubahan tracked lokal ke commit terakhir | **Restore to HEAD** (merusak dirty work) |
| Bagikan history ke tim | **Push** / pull request |

Lihat [Undo / Revert agent](undo-revert) untuk perbandingan penuh. Aturan jempol: perbaiki turn agent jelek dengan **Revert agent** dulu; pakai git saat peduli history dan remote.

## Tips

- Commit sebelum refactor agent besar supaya **Restore to HEAD** punya baseline bersih.
- Prefer commit kecil setelah turn bagus daripada satu blob “wip agent”.
- Baca diff meski ringkasan chat terdengar sempurna.
- CLI: `inferenesia` bisa menggerakkan alur git-related dari terminal bila kamu lebih nyaman di sana.

## Kesalahan umum

| Salah | Lebih baik |
|-------|------------|
| Commit tanpa review diff agent | Buka Git diff dulu |
| Restore to HEAD sebagai undo | Pakai **Revert agent** |
| Push secrets | Unstage, gitignore, rotate key |
| Kerja di branch salah | Cek nama branch sebelum commit |

## Lanjut

- [Undo / Revert agent](undo-revert)
- [Tool & shell](tools-shell) — terminal di samping Git
- [Studi kasus](case-studies)
