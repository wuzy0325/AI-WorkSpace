package backend

import _ "embed"

// 嵌入用户说明书 HTML 文件，打包后无需依赖外部文件
//
//go:embed docs/用户说明书.html
var helpDocContent string
