<div align="center">
  <img src="https://raw.githubusercontent.com/OpenListTeam/Logo/main/logo.svg" width="128" height="128" alt="logo" />

  <p><em>OpenList is a resilient, long-term governance, community-driven fork of AList — built to defend open source against trust-based attacks.</em></p>

  <img src="https://goreportcard.com/badge/github.com/OpenListTeam/OpenList/v3" alt="latest version" />
  <a href="https://github.com/OpenListTeam/OpenList/blob/main/LICENSE"><img src="https://img.shields.io/github/license/OpenListTeam/OpenList" alt="License" /></a>
  <a href="https://github.com/OpenListTeam/OpenList/actions?query=workflow%3ABuild"><img src="https://img.shields.io/github/actions/workflow/status/OpenListTeam/OpenList/build.yml?branch=main" alt="Build status" /></a>
  <a href="https://github.com/OpenListTeam/OpenList/releases"><img src="https://img.shields.io/github/release/OpenListTeam/OpenList" alt="latest version" /></a>

  <a href="https://github.com/OpenListTeam/OpenList/discussions"><img src="https://img.shields.io/github/discussions/OpenListTeam/OpenList?color=%23ED8936" alt="discussions" /></a>
  <a href="https://github.com/OpenListTeam/OpenList/releases"><img src="https://img.shields.io/github/downloads/OpenListTeam/OpenList/total?color=%239F7AEA&logo=github" alt="Downloads" /></a>
</div>

---

- English | [中文](./README/README_cn.md) | [日本語](./README/README_ja.md) | [Dutch](./README/README_nl.md) | [한국어](./README/README_ko.md) | [Deutsch](./README/README_de.md) | [Русский](./README/README_ru.md) | [Français](./README/README_fr.md) | [Español](./README/README_es.md) | [العربية](./README/README_ar.md)

- [Contributing](./CONTRIBUTING.md)
- [CODE OF CONDUCT](./CODE_OF_CONDUCT.md)
- [LICENSE](./LICENSE)

## 一键安装（交互式管理脚本）

本仓库提供安装/管理脚本 [installer/openlist.sh](./installer/openlist.sh)，安装本仓库构建的版本（GitHub Releases + Docker 镜像 `anyear/openlist`）。

### 安装管理脚本

Linux / macOS / WSL：

```bash
sudo curl -fsSL "https://raw.githubusercontent.com/sucooer/OpenList/refs/heads/selfuse/installer/openlist.sh" -o /usr/local/bin/openlist
sudo chmod +x /usr/local/bin/openlist
openlist
```

Android Termux（`/usr/local/bin` 换成 `$PREFIX/bin`）：

```bash
curl -fsSL "https://raw.githubusercontent.com/sucooer/OpenList/refs/heads/selfuse/installer/openlist.sh" -o $PREFIX/bin/openlist
chmod +x $PREFIX/bin/openlist
openlist
```

运行 `openlist` 唤出交互式菜单。

### 安装（二进制模式，推荐）

1. 菜单选 `1`（安装 OpenList）；
2. 选版本：菜单默认的 `beta` 可用；正式版发布后选 `1` 自动跟踪 `releases/latest`，或选 `3` 手动输入版本标签（如 `beta`）；
3. 确认后脚本会下载 `openlist-linux-<架构>.tar.gz`、创建 systemd 服务并启动。

初始管理员密码在菜单 `9`（查看日志）里，服务首次启动时会打印。服务默认监听 `5244` 端口。

### 安装（Docker 模式）

1. 菜单选 `11`（Docker 一键安装/启动）；
2. 选择镜像标签：`beta` / `beta-ffmpeg` / `beta-aio` / `beta-aria2`，或手动输入（正式版发布后可输入 `latest` 等）；
3. 脚本会拉取 `anyear/openlist:<标签>` 并创建容器（端口 `5244`）。

### 其他菜单功能

- `2` 更新 / `3` 卸载（二进制），`12`-`16` 容器进入、改密码、重启、状态、日志
- `4` 从 Alist 迁移数据，`5`-`8` 服务启停/状态/日志，`10` 管理员密码管理
- `17` Nginx 域名绑定/反代，`18` 及以后：定时自动更新（二进制与 Docker）

## Disclaimer

OpenList is an open-source project independently maintained by the OpenList Team, following the AGPL-3.0 license and committed to maintaining complete code openness and modification transparency.

We have noticed the emergence of some third-party projects in the community with names similar to this project, such as OpenListApp/OpenListApp, as well as some paid proprietary software using the same or similar naming. To avoid user confusion, we hereby declare:

- OpenList has no official association with any third-party derivative projects.

- All software, code, and services of this project are maintained by the OpenList Team and are freely available on GitHub.

- Project documentation and API services primarily rely on charitable resources provided by Cloudflare. There are currently no paid plans or commercial deployments, and the use of existing features does not involve any costs.

We respect the community's rights to free use and derivative development, but we also strongly urge downstream projects:

- Should not use the "OpenList" name for impersonation promotion or commercial gain;

- Must not distribute OpenList-based code in a closed-source manner or violate AGPL license terms.

To better maintain healthy ecosystem development, we recommend:

- Clearly indicate the project source and choose appropriate open-source licenses in accordance with the open-source spirit;

- If involving commercial use, please avoid using "OpenList" or any confusing naming as the project name;

- If you need to use materials located under OpenListTeam/Logo, you may modify and use them under compliance with the agreement.

Thank you for your support and understanding of the OpenList project.

## Features

- [x] Multiple storages
  - [x] Local storage
  - [x] [Aliyundrive](https://www.alipan.com)
  - [x] OneDrive / Sharepoint ([Global](https://www.microsoft.com/en-us/microsoft-365/onedrive/online-cloud-storage), [CN](https://portal.partner.microsoftonline.cn), DE, US)
  - [x] [189cloud](https://cloud.189.cn) (Personal, Family)
  - [x] [GoogleDrive](https://drive.google.com)
  - [x] [123pan](https://www.123pan.com)
  - [x] [FTP / SFTP](https://en.wikipedia.org/wiki/File_Transfer_Protocol)
  - [x] [PikPak](https://www.mypikpak.com)
  - [x] [S3](https://aws.amazon.com/s3)
  - [x] [Seafile](https://seafile.com)
  - [x] [UPYUN Storage Service](https://www.upyun.com/products/file-storage)
  - [x] [WebDAV](https://en.wikipedia.org/wiki/WebDAV)
  - [x] Teambition([China](https://www.teambition.com), [International](https://us.teambition.com))
  - [x] [MediaFire](https://www.mediafire.com)
  - [x] [Mediatrack](https://www.mediatrack.cn)
  - [x] [ProtonDrive](https://proton.me/drive)
  - [x] [139yun](https://yun.139.com) (Personal, Family, Group, Share)
  - [x] [YandexDisk](https://disk.yandex.com)
  - [x] [BaiduNetdisk](http://pan.baidu.com)
  - [x] [Terabox](https://www.terabox.com/main)
  - [x] [UC](https://drive.uc.cn)
  - [x] [Quark](https://pan.quark.cn)
  - [x] [Thunder](https://pan.xunlei.com)
  - [x] [Lanzou](https://www.lanzou.com)
  - [x] [ILanzou](https://www.ilanzou.com)
  - [x] [Google photo](https://photos.google.com)
  - [x] [Mega.nz](https://mega.nz)
  - [x] [Baidu photo](https://photo.baidu.com)
  - [x] [SMB](https://en.wikipedia.org/wiki/Server_Message_Block)
  - [x] [115](https://115.com)
  - [X] [Cloudreve](https://cloudreve.org)
  - [x] [Dropbox](https://www.dropbox.com)
  - [x] [FeijiPan](https://www.feijipan.com)
  - [x] [dogecloud](https://www.dogecloud.com/product/oss)
  - [x] [Azure Blob Storage](https://azure.microsoft.com/products/storage/blobs)
  - [x] [Chaoxing](https://www.chaoxing.com)
  - [x] [CNB](https://cnb.cool/)
  - [x] [Degoo](https://degoo.com)
  - [x] [Doubao](https://www.doubao.com)
  - [x] [Febbox](https://www.febbox.com)
  - [x] [GitHub](https://github.com)
  - [x] [OpenList](https://github.com/OpenListTeam/OpenList)
  - [x] [Teldrive](https://github.com/tgdrive/teldrive)
  - [x] [Weiyun](https://www.weiyun.com)
  - [x] [DingTalk Docs](https://alidocs.dingtalk.com/)
- [x] Easy to deploy and out-of-the-box
- [x] File preview (PDF, markdown, code, plain text, ...)
- [x] Image preview in gallery mode
- [x] Video and audio preview, support lyrics and subtitles
- [x] Office documents preview (docx, pptx, xlsx, ...)
- [x] `README.md` preview rendering
- [x] File permalink copy and direct file download
- [x] Dark mode
- [x] I18n
- [x] Protected routes (password protection and authentication)
- [x] WebDAV
- [x] Docker Deploy
- [x] Cloudflare Workers proxy
- [x] File/Folder package download
- [x] Web upload(Can allow visitors to upload), delete, mkdir, rename, move and copy
- [x] Offline download
- [x] Copy files between two storage
- [x] Multi-thread downloading acceleration for single-thread download/stream

## Document

- 📘 [Docs](https://doc.oplist.org)
- 🌏 [CN Mirror](https://doc.oplist.org.cn)
- ⚖️ [Terms of Use](https://doc.oplist.org/terms)
- 🔒 [Privacy Policy](https://doc.oplist.org/privacy)

## Demo

- 🌎 [Global Demo](https://demo.oplist.org)
- 🇨🇳 [CN Demo](https://demo.oplist.org.cn)

## Discussion

Please refer to [*Discussions*](https://github.com/OpenListTeam/OpenList/discussions) for raising general questions, ***Issues* is for bug reports and feature requests only.**

## Sponsor

[![VPS.Town](https://vps.town/static/images/sponsor.png)](https://vps.town "VPS.Town - Trust, Effortlessly. Your Cloud, Reimagined.")

## Donors

Thanks to the following donors for their generous support:

- [HisAtri](https://github.com/HisAtri)
- 爱发电用户_7jTh
- suka

## License

The `OpenList` is open-source software licensed under the [AGPL-3.0](https://www.gnu.org/licenses/agpl-3.0.txt) license.

## Disclaimer

- This project is a free and open-source software designed to facilitate file sharing via net disks, primarily intended to support the downloading and learning of the Go programming language.
- Please comply with all applicable laws and regulations when using this software. Any form of misuse is strictly prohibited.
- The software is based on official SDKs or APIs without any modification, disruption, or interference with their behavior.
- It only performs HTTP 302 redirects or traffic forwarding, and does not intercept, store, or tamper with any user data.
- This project is not affiliated with any official platform or service provider.
- The software is provided "as is", without any warranties of any kind, either express or implied, including but not limited to warranties of merchantability or fitness for a particular purpose.
- The maintainers are not liable for any direct or indirect damages arising from the use of, or inability to use, this software.
- You are solely responsible for any risks associated with using this software, including but not limited to account bans or download speed limitations.
- This project is licensed under the [AGPL-3.0](https://www.gnu.org/licenses/agpl-3.0.txt) License. Please see the [LICENSE](./LICENSE) file for details.

## Contact Us

- [@GitHub](https://github.com/OpenListTeam)
- [Telegram Group](https://t.me/OpenListTeam)
- [Telegram Channel](https://t.me/OpenListOfficial)

## Contributors

We sincerely thank the author [Xhofe](https://github.com/Xhofe) of the original project [AlistGo/alist](https://github.com/AlistGo/alist) and all other contributors.

Thanks goes to these wonderful people:

[![Contributors](https://contrib.rocks/image?repo=OpenListTeam/OpenList)](https://github.com/OpenListTeam/OpenList/graphs/contributors)

