# Checks gunim's UI Automation provider through Windows' own client,
# the interface Narrator reads through. Start the controls example,
# leave it alone, and run this from another window:
#
#   go run ./example/controls
#   powershell -ExecutionPolicy Bypass -File driver\desktop\testdata\uia-check.ps1
#
# It prints the window's tree, then one PASS or FAIL line per check,
# and writes the same to uia-check.log in the current folder. It
# presses Tab in the example, so keep the keyboard and mouse still
# while it runs, about a minute.

$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName UIAutomationClient, UIAutomationTypes, WindowsBase, System.Windows.Forms

$refs = @(
    [System.Windows.Automation.AutomationElement].Assembly.Location,
    [System.Windows.Automation.ControlType].Assembly.Location,
    [System.Windows.Rect].Assembly.Location
)
Add-Type -ReferencedAssemblies $refs -TypeDefinition @"
using System;
using System.Collections.Concurrent;
using System.Runtime.InteropServices;
using System.Windows.Automation;
public static class Keys {
    [DllImport("user32.dll")] static extern void keybd_event(byte vk, byte scan, uint flags, UIntPtr extra);
    [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hwnd);
    // Tab, with its scan code, as a keyboard sends it.
    public static void Tab() {
        keybd_event(0x09, 0x0F, 0, UIntPtr.Zero);
        keybd_event(0x09, 0x0F, 2, UIntPtr.Zero);
    }
}
public static class FocusLog {
    public static ConcurrentQueue<string> Seen = new ConcurrentQueue<string>();
    public static void Start() {
        Automation.AddAutomationFocusChangedEventHandler((s, e) => {
            var el = s as AutomationElement;
            try {
                Seen.Enqueue(el.Current.ControlType.ProgrammaticName + " '" + el.Current.Name + "' " + el.Current.FrameworkId);
            } catch {
                Seen.Enqueue("(gone)");
            }
        });
    }
}
"@

$AE = [System.Windows.Automation.AutomationElement]
$CT = [System.Windows.Automation.ControlType]
$Scope = [System.Windows.Automation.TreeScope]
$log = Join-Path (Get-Location) 'uia-check.log'
Set-Content -Path $log -Value "uia-check $(Get-Date -Format s)"
$failures = 0

function Say($s) { Write-Host $s; Add-Content -Path $log -Value $s }
function Check($name, $ok, $detail) {
    if ($ok) { Say "PASS $name$(if ($detail) { ": $detail" })" }
    else { Say "FAIL $name$(if ($detail) { ": $detail" })"; $script:failures++ }
}
function Wait-For($what, [scriptblock]$fn, $seconds = 10) {
    $until = (Get-Date).AddSeconds($seconds)
    while ((Get-Date) -lt $until) {
        try { $r = & $fn; if ($r) { return $r } } catch { }
        Start-Sleep -Milliseconds 200
    }
    Say "  (gave up waiting for $what)"
    return $null
}
function Find($type, $name) {
    $conds = @(New-Object System.Windows.Automation.PropertyCondition($AE::ControlTypeProperty, $type))
    if ($name) { $conds += New-Object System.Windows.Automation.PropertyCondition($AE::NameProperty, $name) }
    $cond = if ($conds.Count -eq 1) { $conds[0] } else { New-Object System.Windows.Automation.AndCondition($conds) }
    return $script:win.FindFirst($Scope::Descendants, $cond)
}
function Dump($el, $depth) {
    $walker = [System.Windows.Automation.TreeWalker]::RawViewWalker
    $c = $el.Current
    $pats = ($el.GetSupportedPatterns() | ForEach-Object { $_.ProgrammaticName -replace 'PatternIdentifiers.Pattern', '' }) -join ','
    Say ("{0}{1} '{2}' [{3}] {4}" -f ('  ' * $depth), $c.ControlType.ProgrammaticName, $c.Name, $pats, $c.BoundingRectangle)
    if ($depth -ge 12) { return }
    $k = $walker.GetFirstChild($el)
    while ($k) { Dump $k ($depth + 1); $k = $walker.GetNextSibling($k) }
}

# 1. The window, and its tree, which the example publishes a frame
#    after UI Automation first asks.
$script:win = Wait-For 'the window' {
    $AE::RootElement.FindFirst($Scope::Children,
        (New-Object System.Windows.Automation.PropertyCondition($AE::NameProperty, 'gunim controls')))
}
if (-not $win) { Check 'window found' $false 'is the controls example running?'; exit 1 }
$tabs = Wait-For 'the tabs' { Find $CT::TabItem 'Popups' }
Say '--- tree ---'
Dump $win 0
Say '------------'
$combo = Find $CT::ComboBox 'Fruit'
$button = Find $CT::Button 'Switch theme'
Check '1 tree' ($tabs -and $combo -and $button) "tab 'Popups' $([bool]$tabs), combo box 'Fruit' $([bool]$combo), button 'Switch theme' $([bool]$button)"

# 2. Choosing a tab, and checking a checkbox.
$tab = Find $CT::TabItem 'Toggles'
if ($tab) { $tab.GetCurrentPattern([System.Windows.Automation.SelectionItemPattern]::Pattern).Select() }
$check = Wait-For 'the checkbox' { Find $CT::CheckBox 'Send me the newsletter' }
if ($check) {
    $toggle = $check.GetCurrentPattern([System.Windows.Automation.TogglePattern]::Pattern)
    $before = $toggle.Current.ToggleState
    $toggle.Toggle()
    $after = Wait-For 'the checkbox to change' {
        $s = $toggle.Current.ToggleState; if ($s -ne $before) { $s }
    } 3
    Check '2 tab and checkbox' ($null -ne $after) "Toggles tab chosen, checkbox $before -> $after"
} else {
    Check '2 tab and checkbox' $false 'the Toggles tab showed no checkbox'
}

# 3. Setting the slider, and finding the checkbox by a point on
#    screen: the two calls that take floating-point numbers.
$slider = Find $CT::Slider $null
if ($slider) {
    $range = $slider.GetCurrentPattern([System.Windows.Automation.RangeValuePattern]::Pattern)
    $range.SetValue(42)
    $v = Wait-For 'the slider to move' { $x = $range.Current.Value; if ([math]::Abs($x - 42) -lt 0.5) { "$x" } } 3
    $r = $check.Current.BoundingRectangle
    $at = $AE::FromPoint((New-Object System.Windows.Point(($r.X + $r.Width / 2), ($r.Y + $r.Height / 2))))
    Check '3 slider and point' (($null -ne $v) -and ($at.Current.Name -eq 'Send me the newsletter')) "slider now $($range.Current.Value) (range $($range.Current.Minimum)-$($range.Current.Maximum)); element at the checkbox's middle: $($at.Current.ControlType.ProgrammaticName) '$($at.Current.Name)'"
} else {
    Check '3 slider and point' $false 'no slider found'
}

# 4. Focus events, which Narrator follows, as Tab moves focus.
[FocusLog]::Start()
(New-Object -ComObject WScript.Shell).AppActivate('gunim controls') | Out-Null
[Keys]::SetForegroundWindow([IntPtr]$win.Current.NativeWindowHandle) | Out-Null
Start-Sleep -Milliseconds 500
foreach ($i in 1..4) { [Keys]::Tab(); Start-Sleep -Milliseconds 400 }
Start-Sleep -Milliseconds 500
$seen = @([FocusLog]::Seen.ToArray() | Where-Object { $_ -like '* gunim' })
$seen | ForEach-Object { Say "  focus: $_" }
Check '4 focus events' ($seen.Count -ge 3) "$($seen.Count) focus events from gunim elements for 4 presses of Tab"

# 5. Opening and closing the drop-down.
$popups = Find $CT::TabItem 'Popups'
if ($popups) { $popups.GetCurrentPattern([System.Windows.Automation.SelectionItemPattern]::Pattern).Select() }
$combo = Wait-For 'the combo box' { Find $CT::ComboBox 'Fruit' }
if ($combo) {
    $ec = $combo.GetCurrentPattern([System.Windows.Automation.ExpandCollapsePattern]::Pattern)
    $ec.Expand()
    $open = Wait-For 'the list to open' { if ($ec.Current.ExpandCollapseState -eq 'Expanded') { $true } } 3
    # The list is a window of its own, beside the example's.
    $mine = $AE::RootElement.FindAll($Scope::Children,
        (New-Object System.Windows.Automation.PropertyCondition($AE::ProcessIdProperty, $win.Current.ProcessId)))
    $menu = $null
    foreach ($w in $mine) {
        if (-not $menu) {
            $menu = $w.FindFirst($Scope::Subtree,
                (New-Object System.Windows.Automation.PropertyCondition($AE::ControlTypeProperty, $CT::Menu)))
        }
    }
    $items = if ($menu) { @($menu.FindAll($Scope::Children, [System.Windows.Automation.Condition]::TrueCondition) | ForEach-Object { $_.Current.Name }) } else { @() }
    $ec.Collapse()
    $closed = Wait-For 'the list to close' { if ($ec.Current.ExpandCollapseState -eq 'Collapsed') { $true } } 3
    $value = $combo.GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Current.Value
    Check '5 drop-down' ($open -and $closed -and $items.Count -gt 0) "opened $([bool]$open), items '$($items -join "', '")', closed $([bool]$closed), value '$value'"
} else {
    Check '5 drop-down' $false 'no combo box found'
}

Say "$failures failed"
exit $failures
