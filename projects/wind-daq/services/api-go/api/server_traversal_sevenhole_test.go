package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	coreinterp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/usecase"
)

// sevenHolePrbFixtureDir 定位七孔对拍夹具目录（tasks-seven-hole-traversal Task 6 产物）。
func sevenHolePrbFixtureDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "..", "..",
		"shared", "algorithms", "go", "sevenhole", "interpolation", "testdata", "prb")
	if _, err := os.Stat(filepath.Join(dir, "7.prb")); err != nil {
		t.Skipf("seven-hole fixture set not available: %v", err)
	}
	return dir
}

// fakeFiveHoleInterpolator 是五孔 Interpolator 的测试替身（返回固定结果）。
type fakeFiveHoleInterpolator struct{}

func (fakeFiveHoleInterpolator) IsLoaded() bool                     { return true }
func (fakeFiveHoleInterpolator) GetValidRange() coreinterp.PrbValidRange {
	return coreinterp.PrbValidRange{AlphaMin: -30, AlphaMax: 30, BetaMin: -30, BetaMax: 30}
}
func (fakeFiveHoleInterpolator) Calculate(coreinterp.InterpolationInput) (coreinterp.InterpolationResult, error) {
	return coreinterp.InterpolationResult{
		IsValid: true, Alpha: 1.25, Beta: -0.75,
		TotalPressure: 500, StaticPressure: 400, MachNumber: 0.15, Velocity: 50,
	}, nil
}

func newTraversalRouter(mgr *usecase.TraversalManager) http.Handler {
	return NewRouter(Deps{TraversalManager: mgr})
}

func postJSON(t *testing.T, router http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// sevenHoleImportBody 用 json.Marshal 构造导入请求体（Windows 路径反斜杠需转义）。
func sevenHoleImportBody(t *testing.T, innerPath string, outerPaths []string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"innerFilePath":  innerPath,
		"outerFilePaths": outerPaths,
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return string(payload)
}

// sevenHoleFixturePaths 返回夹具内区与六扇区路径。
func sevenHoleFixturePaths(dir string) (string, []string) {
	outer := make([]string, 0, 6)
	for i := 1; i <= 6; i++ {
		outer = append(outer, filepath.Join(dir, fmt.Sprintf("%d.prb", i)))
	}
	return filepath.Join(dir, "7.prb"), outer
}

// TestImportSevenHolePrb 导入 7 份 .prb 成功：返回逐文件信息（内区 169、扇区 52）
// 与 validRange（spec §5.6 契约）。
func TestImportSevenHolePrb(t *testing.T) {
	dir := sevenHolePrbFixtureDir(t)
	mgr := usecase.NewTraversalManager(nil, nil, nil, nil, nil)
	router := newTraversalRouter(mgr)

	inner, outer := sevenHoleFixturePaths(dir)
	w := postJSON(t, router, "/api/traversal/importSevenHolePrb", sevenHoleImportBody(t, inner, outer))
	if w.Code != http.StatusOK {
		t.Fatalf("importSevenHolePrb = %d, want 200: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Files []struct {
			FilePath   string `json:"filePath"`
			FileName   string `json:"fileName"`
			Sector     int    `json:"sector"`
			PointCount int    `json:"pointCount"`
			LoadedAt   int64  `json:"loadedAt"`
		} `json:"files"`
		ValidRange struct {
			AlphaMin float64 `json:"alphaMin"`
			AlphaMax float64 `json:"alphaMax"`
			BetaMin  float64 `json:"betaMin"`
			BetaMax  float64 `json:"betaMax"`
		} `json:"validRange"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Files) != 7 {
		t.Fatalf("files count = %d, want 7", len(resp.Files))
	}
	if resp.Files[0].Sector != 7 || resp.Files[0].PointCount != 169 {
		t.Errorf("inner file info = %+v, want sector 7 / 169 points", resp.Files[0])
	}
	for i := 1; i <= 6; i++ {
		if resp.Files[i].Sector != i || resp.Files[i].PointCount != 52 {
			t.Errorf("outer file info = %+v, want sector %d / 52 points", resp.Files[i], i)
		}
	}
	if resp.ValidRange.AlphaMin != -30 || resp.ValidRange.AlphaMax != 30 ||
		resp.ValidRange.BetaMin != -30 || resp.ValidRange.BetaMax != 30 {
		t.Errorf("validRange = %+v, want ±30", resp.ValidRange)
	}
}

// TestImportSevenHolePrb_Errors 缺内区路径、扇区数量错误、文件不存在均返回 400 且消息明确。
func TestImportSevenHolePrb_Errors(t *testing.T) {
	dir := sevenHolePrbFixtureDir(t)
	router := newTraversalRouter(usecase.NewTraversalManager(nil, nil, nil, nil, nil))

	inner, outer := sevenHoleFixturePaths(dir)
	missingBody := sevenHoleImportBody(t, inner, []string{
		outer[0], outer[1], filepath.Join(dir, "missing.prb"), outer[3], outer[4], outer[5],
	})
	cases := []struct {
		name string
		body string
		want string
	}{
		{"empty inner", sevenHoleImportBody(t, "", outer), "innerFilePath"},
		{"wrong outer count", sevenHoleImportBody(t, "7.prb", []string{"1.prb"}), "6 份"},
		{"missing file", missingBody, "missing.prb"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := postJSON(t, router, "/api/traversal/importSevenHolePrb", tc.body)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", w.Code)
			}
			if !strings.Contains(w.Body.String(), tc.want) {
				t.Errorf("error %q must contain %q", w.Body.String(), tc.want)
			}
		})
	}
}

// importSevenHolePrbOK 测试辅助：通过 API 导入七孔夹具并保存七孔配置。
func importSevenHolePrbOK(t *testing.T, router http.Handler, dir string) {
	t.Helper()
	inner, outer := sevenHoleFixturePaths(dir)
	if w := postJSON(t, router, "/api/traversal/importSevenHolePrb", sevenHoleImportBody(t, inner, outer)); w.Code != http.StatusOK {
		t.Fatalf("importSevenHolePrb: %s", w.Body.String())
	}
	if w := postJSON(t, router, "/api/traversal/config", `{"probeType":"seven-hole"}`); w.Code != http.StatusOK {
		t.Fatalf("saveConfig: %s", w.Body.String())
	}
}

// TestCalculateRealtimeSevenHole 七孔实时计算全链路：
// importSevenHolePrb → saveConfig(probeType) → calculateRealtime(P1..P7 body)。
func TestCalculateRealtimeSevenHole(t *testing.T) {
	dir := sevenHolePrbFixtureDir(t)
	mgr := usecase.NewTraversalManager(nil, nil, nil, nil, nil)
	router := newTraversalRouter(mgr)
	importSevenHolePrbOK(t, router, dir)

	body := `{"probeType": "seven-hole", "pressures": {"P1": 1000, "P2": 1000, "P3": 1000, "P4": 1000, "P5": 1000, "P6": 1000, "P7": 1500, "Patm": 101325, "Tatm": 20}}`
	w := postJSON(t, router, "/api/traversal/calculateRealtime", body)
	if w.Code != http.StatusOK {
		t.Fatalf("calculateRealtime = %d, want 200: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Alpha      float64 `json:"alpha"`
		Beta       float64 `json:"beta"`
		Pt         float64 `json:"P0"`
		Ps         float64 `json:"Ps"`
		Mach       float64 `json:"machNumber"`
		Velocity   float64 `json:"velocity"`
		Dynamic    float64 `json:"dynamicPressure"`
		IsValid    bool    `json:"isValid"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.IsValid {
		t.Fatalf("expected valid result, got %+v", resp)
	}
	// ka=kb=0 → 与夹具构造原点用例同格：alpha≈-0.5545, beta≈0.3653,
	// pt≈1499.76, ps≈938.61。
	if resp.Alpha < -0.56 || resp.Alpha > -0.55 {
		t.Errorf("alpha = %v, want ≈ -0.5545", resp.Alpha)
	}
	if resp.Beta < 0.36 || resp.Beta > 0.37 {
		t.Errorf("beta = %v, want ≈ 0.3653", resp.Beta)
	}
	if resp.Pt < 1499 || resp.Pt > 1500 || resp.Ps < 938 || resp.Ps > 939 {
		t.Errorf("Pt/Ps = (%v,%v), want ≈ (1499.76,938.61)", resp.Pt, resp.Ps)
	}
	if resp.Dynamic <= 0 || resp.Velocity <= 0 || resp.Mach <= 0 {
		t.Errorf("dynamic/velocity/mach must be positive: %+v", resp)
	}
}

// TestCalculateRealtimeSevenHole_Errors 缺 probeType、缺 P6/P7、类型不一致分别 400；
// P6/P7=0 不被当成缺失（spec §5.6 present-zero 语义）。
func TestCalculateRealtimeSevenHole_Errors(t *testing.T) {
	dir := sevenHolePrbFixtureDir(t)

	t.Run("missing probeType on seven-hole manager", func(t *testing.T) {
		mgr := usecase.NewTraversalManager(nil, nil, nil, nil, nil)
		router := newTraversalRouter(mgr)
		importSevenHolePrbOK(t, router, dir)
		body := `{"pressures": {"P1": 100, "P2": 100, "P3": 100, "P4": 100, "P5": 100, "P6": 100, "P7": 100, "Patm": 101325, "Tatm": 20}}`
		if w := postJSON(t, router, "/api/traversal/calculateRealtime", body); w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("missing P6/P7", func(t *testing.T) {
		mgr := usecase.NewTraversalManager(nil, nil, nil, nil, nil)
		router := newTraversalRouter(mgr)
		importSevenHolePrbOK(t, router, dir)
		body := `{"probeType": "seven-hole", "pressures": {"P1": 100, "P2": 100, "P3": 100, "P4": 100, "P5": 100, "Patm": 101325, "Tatm": 20}}`
		w := postJSON(t, router, "/api/traversal/calculateRealtime", body)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "P6/P7") {
			t.Fatalf("status = %d body = %s, want 400 naming P6/P7", w.Code, w.Body.String())
		}
	})

	t.Run("probeType mismatch", func(t *testing.T) {
		mgr := usecase.NewTraversalManager(nil, nil, nil, nil, nil)
		router := newTraversalRouter(mgr)
		importSevenHolePrbOK(t, router, dir)
		// 切回五孔配置后请求七孔 → 拒绝。
		if w := postJSON(t, router, "/api/traversal/config", `{"probeType":"five-hole"}`); w.Code != http.StatusOK {
			t.Fatal(w.Body.String())
		}
		body := `{"probeType": "seven-hole", "pressures": {"P1": 1, "P2": 1, "P3": 1, "P4": 1, "P5": 1, "P6": 1, "P7": 1, "Patm": 101325, "Tatm": 20}}`
		if w := postJSON(t, router, "/api/traversal/calculateRealtime", body); w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("P6/P7 zero is present not missing", func(t *testing.T) {
		mgr := usecase.NewTraversalManager(nil, nil, nil, nil, nil)
		router := newTraversalRouter(mgr)
		importSevenHolePrbOK(t, router, dir)
		// P1..P5=-1000 使 pAvg<0，P6=P7=0 时 pt>ps（与夹具 P7=0 构造用例同格）。
		body := `{"probeType": "seven-hole", "pressures": {"P1": -1000, "P2": -1000, "P3": -1000, "P4": -1000, "P5": -1000, "P6": 0, "P7": 0, "Patm": 101325, "Tatm": 20}}`
		w := postJSON(t, router, "/api/traversal/calculateRealtime", body)
		if w.Code != http.StatusOK {
			t.Fatalf("P6/P7=0 must be accepted as present, got %d: %s", w.Code, w.Body.String())
		}
	})
}

// sevenHoleCalCsvDir 定位七孔校准 CSV 数据目录（与 W532 数据集同构的校准导出）。
func sevenHoleCalCsvDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "..", "..",
		"projects", "wind-daq", "docs", "7-hole-cal-data")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("seven-hole calibration csv data not available: %v", err)
	}
	return dir
}

// TestImportSevenHoleCalibrationCsv 校准 CSV 导入 action：
// 200 返回 7 文件信息 + validRange，随后七孔实时计算可走通（CSV → 插值网格全链路）。
func TestImportSevenHoleCalibrationCsv(t *testing.T) {
	dir := sevenHoleCalCsvDir(t)
	const stem = "W532.202608.P.7H.1-01-85米每秒（0.242Ma）"
	inner := filepath.Join(dir, stem+"(小角度区).csv")
	outer := make([]string, 0, 6)
	for i := 1; i <= 6; i++ {
		outer = append(outer, filepath.Join(dir, fmt.Sprintf("%s(大角度%d区).csv", stem, i)))
	}
	mgr := usecase.NewTraversalManager(nil, nil, nil, nil, nil)
	router := newTraversalRouter(mgr)

	w := postJSON(t, router, "/api/traversal/importSevenHoleCalibrationCsv", sevenHoleImportBody(t, inner, outer))
	if w.Code != http.StatusOK {
		t.Fatalf("importSevenHoleCalibrationCsv = %d, want 200: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Files []struct {
			Sector     int `json:"sector"`
			PointCount int `json:"pointCount"`
		} `json:"files"`
		ValidRange struct {
			AlphaMin float64 `json:"alphaMin"`
			AlphaMax float64 `json:"alphaMax"`
		} `json:"validRange"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Files) != 7 || resp.Files[0].Sector != 7 || resp.Files[0].PointCount != 169 {
		t.Fatalf("files = %+v, want 7 entries with inner sector 7 / 169 pts", resp.Files)
	}
	for i := 1; i <= 6; i++ {
		if resp.Files[i].Sector != i || resp.Files[i].PointCount != 52 {
			t.Errorf("files[%d] = %+v, want sector %d / 52 pts", i, resp.Files[i], i)
		}
	}
	if resp.ValidRange.AlphaMin != -30 || resp.ValidRange.AlphaMax != 30 {
		t.Errorf("validRange = %+v, want ±30", resp.ValidRange)
	}

	// 保存七孔配置后实时计算走通（构造原点用例同格）。
	if w := postJSON(t, router, "/api/traversal/config", `{"probeType":"seven-hole"}`); w.Code != http.StatusOK {
		t.Fatal(w.Body.String())
	}
	body := `{"probeType": "seven-hole", "pressures": {"P1": 1000, "P2": 1000, "P3": 1000, "P4": 1000, "P5": 1000, "P6": 1000, "P7": 1500, "Patm": 101325, "Tatm": 20}}`
	w = postJSON(t, router, "/api/traversal/calculateRealtime", body)
	if w.Code != http.StatusOK {
		t.Fatalf("calculateRealtime after csv import = %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"isValid":true`) {
		t.Errorf("expected valid result after csv import, got %s", w.Body.String())
	}
}

// TestCalculateRealtimeFiveHoleCompat 旧五孔 body（无 probeType）响应与现行一致。
func TestCalculateRealtimeFiveHoleCompat(t *testing.T) {
	mgr := usecase.NewTraversalManager(nil, nil, nil, nil, nil)
	mgr.SetInterpolator(fakeFiveHoleInterpolator{})
	router := newTraversalRouter(mgr)

	body := `{"pressures": {"P1": 100, "P2": 100, "P3": 100, "P4": 100, "P5": 100, "Patm": 101325, "Tatm": 20}}`
	w := postJSON(t, router, "/api/traversal/calculateRealtime", body)
	if w.Code != http.StatusOK {
		t.Fatalf("five-hole calculateRealtime = %d, want 200: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["alpha"] != 1.25 || resp["beta"] != -0.75 || resp["P0"] != 500.0 || resp["Ps"] != 400.0 || resp["isValid"] != true {
		t.Errorf("five-hole response changed: %v", resp)
	}
}

// TestClearInterpolator 五孔/七孔各一例清理成功；缺失/未知 probeType 返回 400。
func TestClearInterpolator(t *testing.T) {
	t.Run("seven-hole", func(t *testing.T) {
		dir := sevenHolePrbFixtureDir(t)
		mgr := usecase.NewTraversalManager(nil, nil, nil, nil, nil)
		router := newTraversalRouter(mgr)
		importSevenHolePrbOK(t, router, dir)
		if !mgr.HasLoadedInterpolator() {
			t.Fatal("precondition: seven-hole interpolator must be loaded")
		}
		w := postJSON(t, router, "/api/traversal/clearInterpolator", `{"probeType":"seven-hole"}`)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"cleared":true`) {
			t.Fatalf("clearInterpolator = %d %s, want 200 cleared", w.Code, w.Body.String())
		}
		if mgr.HasLoadedInterpolator() {
			t.Error("seven-hole interpolator must be cleared")
		}
	})

	t.Run("five-hole", func(t *testing.T) {
		mgr := usecase.NewTraversalManager(nil, nil, nil, nil, nil)
		mgr.SetInterpolator(fakeFiveHoleInterpolator{})
		router := newTraversalRouter(mgr)
		if !mgr.HasLoadedInterpolator() {
			t.Fatal("precondition: five-hole interpolator must be loaded")
		}
		w := postJSON(t, router, "/api/traversal/clearInterpolator", `{"probeType":"five-hole"}`)
		if w.Code != http.StatusOK {
			t.Fatalf("clearInterpolator = %d, want 200: %s", w.Code, w.Body.String())
		}
		if mgr.HasLoadedInterpolator() {
			t.Error("five-hole interpolator must be cleared")
		}
	})

	t.Run("missing probeType", func(t *testing.T) {
		router := newTraversalRouter(usecase.NewTraversalManager(nil, nil, nil, nil, nil))
		if w := postJSON(t, router, "/api/traversal/clearInterpolator", `{}`); w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("unknown probeType", func(t *testing.T) {
		router := newTraversalRouter(usecase.NewTraversalManager(nil, nil, nil, nil, nil))
		w := postJSON(t, router, "/api/traversal/clearInterpolator", `{"probeType":"nine-hole"}`)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})
}

// 编译期引用检查：traversal 包常量与 usecase 方法的契约保持可用。
var (
	_ = traversal.ProbeTypeSevenHole
	_ = (*usecase.TraversalManager).CalculateSevenHoleRealtime
)
