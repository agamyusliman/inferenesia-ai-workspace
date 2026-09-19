---
title: Undo / Revert agent
category: safety
summary: Undo vs Revert agent vs Restore to HEAD pada file dirty, plus alur kerja sederhana
keywords: undo redo revert agent restore head safety timeline dirty files
order: 30
---

# Undo / Revert agent

Saat agent mengedit file, Inferenesia menyimpan jaring pengaman supaya kerja uncommitted bisa kembali. Label penting: **Undo**, **Revert agent**, dan **Restore to HEAD** beda fungsi.

## Kenapa ini ada

Pola gagal yang sering:

1. Kamu edit `auth.ts` (belum commit).
2. Agent rewrite `auth.ts` dan menyentuh file lain.
3. Kamu `git restore` dengan niat “undo agent”.
4. Edit kotor milikmu ikut hilang.

Jalur undo agent di Inferenesia mengembalikan **byte di disk sebelum agent menulis**, termasuk dirty edits kamu. Itu bukan “kembali ke commit terakhir”.

## Tiga aksi

| Aksi | Mengembalikan ke | Dirty sebelum agent tetap? | Pakai git? |
|------|------------------|----------------------------|------------|
| **Undo** | Step sebelumnya di stack undo | Ya (itu tujuannya) | Tidak |
| **Redo** | Terapkan lagi yang baru di-undo | Sampai kamu edit di luar stack | Tidak |
| **Revert agent** | Konten pre-agent untuk file di turn agent terakhir | Ya | Tidak |
| **Restore to HEAD** | Working tree ke arah commit terakhir | **Tidak** (bisa hapus dirty work) | Ya |

**Revert agent** dan **Restore to HEAD** sengaja dipisah di UI.

## Toolbar

```text
[Undo] [Redo] [Revert agent] [Restore to HEAD] [Agent changes]
```

- **Undo / Redo** — melangkah di write file agent (dan terkait) untuk workspace ini.
- **Revert agent** — buang hasil turn agent terakhir, kembalikan file dirty pre-turn.
- **Restore to HEAD** — reset bergaya git ke HEAD. Pakai hanya kalau memang itu niatnya.
- **Agent changes** — timeline file yang disentuh agent, biar dicek dulu.

Stack undo **per workspace**. Ganti project tidak mencampur stack.

## Kapan pakai yang mana

| Situasi | Pilih |
|---------|-------|
| Agent baru saja bikin turn multi-file jelek | **Revert agent** |
| Cuma mau mundur satu step write | **Undo** (bisa berulang) |
| Undo kebablasan | **Redo** |
| Mau baseline commit, buang dirty lokal | **Restore to HEAD** (konfirmasi dulu) |
| Mau history di git, bukan cuma undo lokal | **Commit** dulu, baru eksperimen |

## Alur yang disarankan

1. WIP penting: commit di branch, atau setidaknya kamu sadar isinya.
2. Minta agent mengubah sesuatu.
3. Review **Agent changes** atau diff di Git / editor.
4. Kalau turn salah: **Revert agent** (atau Undo bertahap).
5. Kalau bagus: lanjut kerja, atau stage + commit.
6. **Restore to HEAD** hanya saat memang mau baseline commit.

## Undo ≠ git

| Topik | Undo / Revert agent | Git |
|-------|---------------------|-----|
| Baseline | Snapshot sebelum write agent | Commit terakhir (HEAD) / index |
| Scope | File yang ditulis agent | Tree tracked yang kamu pilih |
| Tahan “discard all local” | Ya, sampai stack tertimpa | Tidak |
| Cocok untuk | Salah harian dari agent | History, remote, PR |

Boleh dua-duanya: undo untuk perbaiki turn jelek, commit saat tree sudah oke.

## Tips

- **Revert agent** paling enak langsung setelah turn jelek, sebelum banyak hand-edit di file yang sama.
- Setelah banyak edit manual, stack undo bisa tidak cocok dengan mental model. Cek isi file, bukan cuma nama tombol.
- **Restore to HEAD** merusak dirty work. Anggap alat tajam.
- CLI: `inferenesia undo` (bila tersedia) mengikuti stack workspace yang sama ide-nya dengan desktop.

## Kesalahan umum

| Salah | Perbaikan |
|-------|-----------|
| Restore to HEAD sebagai “undo agent” | Pakai **Revert agent** atau **Undo** |
| Kira Undo membatalkan commit | Tidak; pakai git untuk commit |
| Harap satu undo global semua project | Stack per workspace |

## Lanjut

- [Git](git) — stage, commit, push setelah turn bagus
- [Plan Mode & Mission](plan-mission) — rencanakan dulu sebelum write berisiko
- [Studi kasus](case-studies) — walkthrough edit aman pertama
