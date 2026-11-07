package main

import (
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

// monitorPanel shows the active session's server, as FinalShell's left
// column does.
func (a *App) monitorPanel(c *ui.Context) {
	s := a.current()
	ui.Scroll(c).Fill().Children(func() {
		col := ui.Column(c).FillWidth().Padding(10, 10, 8, 10).Gap(6)
		col.Children(func() {
			if s == nil {
				muted(c, "未连接").Padding(4, 0)
				return
			}
			st := s.stats
			ui.Row(c).Gap(6).Children(func() {
				ip := s.ip
				if ip == "" {
					ip = s.Host.Addr
				}
				ui.Text(c, "IP "+ip).SingleLine().Grow(1).MinWidth(0)
				cp := ui.Text(c, "复制").TextColor(theme.Accent).Cursor(ui.CursorPointer)
				if cp.Clicked() {
					c.WriteClipboard(ip)
				}
			})
			if !st.OK || s.status != statusConnected {
				if s.status == statusConnected {
					muted(c, "正在读取服务器信息…")
				} else {
					muted(c, statusText(s))
				}
				return
			}
			ui.Text(c, fmt.Sprintf("运行 %d 天", int(st.Uptime.Hours()/24)))
			ui.Text(c, fmt.Sprintf("负载 %.2f, %.2f, %.2f", st.Load[0], st.Load[1], st.Load[2]))
			bar(c, "CPU", st.CPU/100, fmt.Sprintf("%.0f%%", st.CPU), ui.Hex("#86efac"))
			bar(c, "内存", ratio(st.MemUsed, st.MemTotal), fmt.Sprintf("%.0f%%  %s/%s", 100*ratio(st.MemUsed, st.MemTotal), formatBytes(float64(st.MemUsed)), formatBytes(float64(st.MemTotal))), ui.Hex("#fdba74"))
			bar(c, "交换", ratio(st.SwapUsed, st.SwapTotal), fmt.Sprintf("%.0f%%  %s/%s", 100*ratio(st.SwapUsed, st.SwapTotal), formatBytes(float64(st.SwapUsed)), formatBytes(float64(st.SwapTotal))), ui.Hex("#93c5fd"))

			procTable(c, st.Procs)

			ui.Row(c).Gap(8).Margin(4, 0, 0, 0).Children(func() {
				ui.Text(c, "↑"+formatBytes(st.TxRate)).TextColor(ui.Hex("#ea580c"))
				ui.Text(c, "↓"+formatBytes(st.RxRate)).TextColor(ui.Hex("#16a34a"))
				ui.Spacer(c)
				muted(c, st.Iface)
			})
			sparkline(c, [][]float64{st.TxHist, st.RxHist}, []ui.Color{ui.Hex("#fb923c"), ui.Hex("#4ade80")}, formatBytes)

			ui.Row(c).Gap(8).Margin(4, 0, 0, 0).Children(func() {
				ui.Text(c, formatLatency(st.Latency)).TextColor(theme.Accent)
				ui.Spacer(c)
				muted(c, "延迟")
			})
			sparkline(c, [][]float64{st.LatHist}, []ui.Color{ui.Hex("#60a5fa")}, func(v float64) string { return fmt.Sprintf("%.0f", v) })

			diskTable(c, st.Disks)
		})
	})
}

func ratio(a, b uint64) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

// bar is a labeled usage bar with its text over it.
func bar(c *ui.Context, label string, frac float64, text string, fill ui.Color) {
	frac = math.Max(0, math.Min(1, frac))
	ui.Row(c).Gap(6).Children(func() {
		ui.Text(c, label).Width(30).Shrink(0)
		ui.Box(c).Grow(1).Height(18).Background(ui.Hex("#e5e7eb")).Border(1, ui.Hex("#d1d5db")).Clip().Children(func() {
			ui.Box(c).Absolute().Top(0).Left(0).Bottom(0).WidthPercent(float32(frac * 100)).Background(fill)
			ui.Row(c).Fill().PaddingX(6).AlignItems(ui.Center).ZIndex(1).Children(func() {
				ui.Text(c, text).FontSize(12).SingleLine()
			})
		})
	})
}

func procTable(c *ui.Context, procs []Proc) {
	ui.Column(c).Border(1, ui.Hex("#d1d5db")).Background(ui.Hex("#fafafa")).Children(func() {
		ui.Row(c).Background(ui.Hex("#60a5fa")).PaddingX(6).Height(20).Children(func() {
			ui.Text(c, "内存").Width(54).TextColor(ui.Hex("#ffffff")).FontSize(12)
			ui.Text(c, "CPU").Width(40).TextColor(ui.Hex("#ffffff")).FontSize(12)
			ui.Text(c, "命令").TextColor(ui.Hex("#ffffff")).FontSize(12)
		})
		for i, p := range procs {
			ui.Row(c.Key(i)).PaddingX(6).Height(19).Children(func() {
				ui.Text(c, formatBytes(float64(p.RSS))).Width(54).FontSize(12)
				ui.Text(c, fmt.Sprintf("%.1f", p.CPU)).Width(40).FontSize(12)
				ui.Text(c, p.Name).FontSize(12).SingleLine().Grow(1).MinWidth(0)
			})
		}
	})
}

func diskTable(c *ui.Context, disks []Disk) {
	ui.Column(c).Margin(6, 0, 0, 0).Border(1, ui.Hex("#d1d5db")).Background(ui.Hex("#ffffff")).Children(func() {
		ui.Row(c).PaddingX(6).Height(20).BorderWidth(0, 0, 1, 0).BorderColor(ui.Hex("#e5e7eb")).Children(func() {
			ui.Text(c, "路径").Grow(1).FontSize(12)
			ui.Text(c, "可用/大小").FontSize(12)
		})
		for i, d := range disks {
			ui.Row(c.Key(i)).PaddingX(6).Height(19).Gap(6).Children(func() {
				ui.Text(c, d.Mount).FontSize(12).SingleLine().Grow(1).MinWidth(0)
				ui.Text(c, formatBytes(float64(d.Avail))+"/"+formatBytes(float64(d.Size))).FontSize(12)
			})
		}
	})
}

// sparkline draws series as bars, the newest on the right, with the
// largest value as a label.
func sparkline(c *ui.Context, series [][]float64, colors []ui.Color, format func(float64) string) {
	peak := 0.0
	for _, s := range series {
		for _, v := range s {
			peak = math.Max(peak, v)
		}
	}
	ui.Box(c).Height(46).Background(ui.Hex("#ffffff")).Border(1, ui.Hex("#e5e7eb")).Draw(func(p *ui.Painter, r ui.Rect) {
		if peak <= 0 {
			return
		}
		step := (r.W - 2) / historyLen
		for si, s := range series {
			for i, v := range s {
				x := r.X + 1 + float32(historyLen-len(s)+i)*step
				h := float32(v/peak) * (r.H - 4)
				w := max(step*0.6, 1)
				if len(series) > 1 {
					w = max(step*0.4, 1)
					x += float32(si) * step * 0.45
				}
				p.Fill(ui.Rect{X: x, Y: r.Y + r.H - 2 - h, W: w, H: h}, colors[si], 0)
			}
		}
	}).Children(func() {
		ui.Text(c, format(peak)).FontSize(10).TextColor(theme.TextMuted).Absolute().Top(1).Left(1).
			Padding(0, 3).Radius(3).Background(ui.RGBA(255, 255, 255, 0.85)).ZIndex(1)
	})
}

// sessionPanel is the list of open sessions, the vertical tabs.
func (a *App) sessionPanel(c *ui.Context) {
	ui.Column(c).Fill().BorderWidth(1, 0, 0, 0).BorderColor(ui.Hex("#d1d5db")).Children(func() {
		ui.Row(c).Padding(6, 6, 6, 10).Gap(4).Children(func() {
			ui.Text(c, fmt.Sprintf("会话 (%d)", len(a.sessions))).Bold().Grow(1)
			if iconButton(c, "plus", "新建连接").Clicked() {
				a.openManager()
			}
		})
		a.sessList.Selected = &a.active
		a.sessList.Key = func(i int) any { return a.sessions[i].ID }
		a.sessList.Label = func(i int) string { return a.sessions[i].label() }
		a.sessList.Reorder = a.moveSessions
		ui.List(c, &a.sessList, len(a.sessions), func(i int) {
			a.sessionRow(c, a.sessions[i], i == a.active)
		}).Grow(1).Padding(0, 4, 4, 4).Children(func() {
			if len(a.sessions) == 0 {
				muted(c, "还没有打开的会话").Padding(8, 10)
			}
		})
	})
}

func (a *App) sessionRow(c *ui.Context, s *Session, active bool) {
	row := ui.Row(c).Gap(8).Padding(5, 4, 5, 8).Radius(5)
	hovered := row.Hovered()
	row.ContextMenu(func(m *ui.Menu) {
		if m.Item("重新连接").Chosen() {
			a.reconnect(s)
		}
		if m.Item("复制会话").Chosen() {
			a.openSession(s.Host)
		}
		if m.Item("断开").Disabled(s.status != statusConnected && s.status != statusConnecting).Chosen() {
			s.disconnect()
			s.status, s.err = statusClosed, "已断开"
		}
		m.Separator()
		if m.Item("关闭").Danger().Chosen() {
			a.closeSession(s)
		}
	})
	if row.DoubleClicked() && s.status != statusConnected && s.status != statusConnecting {
		a.reconnect(s)
	}
	row.Children(func() {
		ui.Box(c).Size(8, 8).Radius(4).Background(s.statusColor(theme)).Shrink(0)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			name := ui.Text(c, s.label()).SingleLine()
			if active {
				name.Bold()
			}
			sub := s.Host.target()
			if s.status != statusConnected {
				sub = statusText(s)
			}
			subColor := theme.TextMuted
			if active {
				subColor = theme.AccentText.Alpha(0.8)
			}
			ui.Text(c, sub).FontSize(11.5).SingleLine().TextColor(subColor)
		})
		closeBtn := iconButton(c, "x", "关闭会话").Size(20, 20)
		if !hovered && !active {
			closeBtn.Invisible()
		}
		if closeBtn.Clicked() {
			a.closeSession(s)
		}
	})
}

func statusText(s *Session) string {
	switch s.status {
	case statusConnecting:
		return "正在连接…"
	case statusFailed:
		return "连接失败"
	case statusClosed:
		if s.err != "" {
			return s.err
		}
		return "已断开"
	}
	return "已连接"
}

func formatLatency(d time.Duration) string {
	ms := float64(d.Microseconds()) / 1000
	if ms < 10 {
		return fmt.Sprintf("%.1fms", ms)
	}
	return fmt.Sprintf("%.0fms", ms)
}
