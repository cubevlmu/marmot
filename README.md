<!-- PROJECT LOGO -->
<br />
<p align="center">
  <a href="https://github.com/cubevlmu/marmot/">
    <img src="images/logo.png" alt="Logo" width="80" height="80">
  </a>

<h3 align="center">Marmot</h3>
  <p align="center">
    Marmot is a modular framework for building QQ group bots based on the OneBot v11 protocol.  
    It provides scheduling, trigger-based actions, logging, and an extensible plugin system.  
    <br />
  </p>

<!-- PROJECT SHIELDS -->
[![Contributors][contributors-shield]][contributors-url]
[![Forks][forks-shield]][forks-url]
[![Stargazers][stars-shield]][stars-url]
[![Issues][issues-shield]][issues-url]
[![MIT License][license-shield]][license-url]
[![LinkedIn][linkedin-shield]][linkedin-url]

## Table of Contents
- [Getting Started](#getting-started)
  - [Prerequisites](#prerequisites)
  - [Installation](#installation)
- [Directory Structure](#directory-structure)
- [Architecture](#architecture)
- [Deployment](#deployment)
- [Built With](#built-with)
- [Contributing](#contributing)
  - [How to Contribute](#how-to-contribute)
- [Versioning](#versioning)
- [Authors](#authors)
- [License](#license)
- [Acknowledgements](#acknowledgements)

---

### Getting Started

Marmot is written in **Go** and designed to integrate with **OneBot v11** for QQ messaging.  
It supports modular extensions, YAML-based configuration, and flexible scheduling.

#### Prerequisites
- Go 1.22 or newer
- SQLite (for `marmot_data.db`)
- A running OneBot v11-compatible adapter (e.g., go-cqhttp)

#### Installation

1. Clone the repository:
   ```sh
   git clone https://github.com/cubevlmu/marmot.git
   cd marmot
   ```

2. Install dependencies:

   ```sh
   go mod tidy
   ```

3. Run the bot:

   ```sh
   go run app/main.go
   ```

---

### Directory Structure

```
marmot/
├── app/                # Application entrypoint (main.go)
├── bot/                # Bot core logic
│   ├── config.yml      # Main configuration
│   ├── scheduler.yml   # Task scheduling rules
│   ├── trigger.yml     # Event triggers
│   ├── filter.yaml     # Message filtering rules
│   ├── marmot_data.db  # SQLite database
│   └── logs/           # Runtime logs
├── core/               # Core framework services
├── modules/            # Modular extensions
├── onebot/             # OneBot v11 protocol implementation
│   └── message/        # Message serialization & parsing
├── utils/              # Utility helpers
├── go.mod              # Go module definition
├── go.sum              # Go dependencies lockfile
└── LICENSE             # MIT License
```

---

### Architecture

Marmot is built with modularity in mind:

* **Core Layer (`core/`)** – Provides essential services such as configuration, logging, and runtime management.
* **Modules (`modules/`)** – Extendable plugins for adding new features without modifying core code.
* **OneBot Integration (`onebot/`)** – Handles QQ group messaging via the OneBot v11 protocol. Modified from [ZeroBot](https://github.com/wdvxdr1123/ZeroBot) project.
* **Bot System (`bot/`)** – Orchestrates configurations, triggers, schedules, and persistent data.

---

### Deployment

Marmot can be deployed alongside any OneBot-compatible adapter (e.g., go-cqhttp).
A basic deployment looks like this:

1. Start your OneBot adapter (go-cqhttp).
2. Configure `bot/config.yml` with your connection details.
3. Run Marmot with:

   ```sh
   go build -o marmot ./app
   ./marmot
   ```

---

### Built With

* [Go](https://go.dev/) – Core programming language
* [OneBot v11](https://onebot.dev/) – Messaging protocol for QQ bots
* [SQLite](https://www.sqlite.org/) – Lightweight database storage
* [YAML](https://yaml.org/) – Configuration format

---

### Contributing

Contributions are what make the open source community great!
Any contributions you make are **greatly appreciated**.

#### How to Contribute

1. Fork the project
2. Create your feature branch (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

---

### Versioning

This project uses **Git** for version control. Check the repository tags for available versions.

---

### Authors

* Cubevlmu – [GitHub](https://github.com/cubevlmu)
* QQ Group Support

See also the list of [contributors][contributors-url] who participated in this project.

---

### License

Distributed under the GPLv2 License. See [LICENSE](LICENSE) for details.
Onebot folder is a embedded version of [ZeroBot](https://github.com/wdvxdr1123/ZeroBot), more information please check the README.md in it.  

---

### Acknowledgements

* [OneBot v11](https://onebot.dev/)
* [go-cqhttp](https://github.com/Mrs4s/go-cqhttp)
* [ZeroBot](https://github.com/wdvxdr1123/ZeroBot)

---

### Special Thanks

* [HappyTBS](https://github.com/happytbs) for testing the framework

---

<!-- links -->

[your-project-path]: cubevlmu/marmot
[contributors-shield]: https://img.shields.io/github/contributors/cubevlmu/marmot.svg?style=flat-square
[contributors-url]: https://github.com/cubevlmu/marmot/graphs/contributors
[forks-shield]: https://img.shields.io/github/forks/cubevlmu/marmot.svg?style=flat-square
[forks-url]: https://github.com/cubevlmu/marmot/network/members
[stars-shield]: https://img.shields.io/github/stars/cubevlmu/marmot.svg?style=flat-square
[stars-url]: https://github.com/cubevlmu/marmot/stargazers
[issues-shield]: https://img.shields.io/github/issues/cubevlmu/marmot.svg?style=flat-square
[issues-url]: https://github.com/cubevlmu/marmot/issues
[license-shield]: https://img.shields.io/github/license/cubevlmu/marmot.svg?style=flat-square
[license-url]: https://github.com/cubevlmu/marmot/blob/master/LICENSE
[linkedin-shield]: https://img.shields.io/badge/-LinkedIn-black.svg?style=flat-square&logo=linkedin&colorB=555
[linkedin-url]: https://linkedin.com/in/your_linkedin_profile