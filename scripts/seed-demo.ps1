# 一键生成演示数据（alice / bob / carol）
# 用法：在 gochat 目录下先启动服务（go run . -db），再执行：
#   powershell -ExecutionPolicy Bypass -File .\scripts\seed-demo.ps1
#
# 会创建：3 个账号、alice-bob 好友、alice-bob 单聊消息、三人群聊

$ErrorActionPreference = "Continue"
$U = "http://127.0.0.1:8080/api/v1"
$BodyFile = Join-Path $env:TEMP "gochat_seed_body.txt"
$ReqFile = Join-Path $env:TEMP "gochat_seed_req.json"

function Req($method, $url, $body, $headers) {
  $cargs = @("-s", "-o", $BodyFile, "-w", "%{http_code}", "-X", $method, $url)
  if ($body) {
    # 用 UTF-8（无 BOM）写请求体：Set-Content -Encoding ASCII 会把中文变成 ?
    [System.IO.File]::WriteAllText($ReqFile, $body, (New-Object System.Text.UTF8Encoding($false)))
    $cargs += @("-H", "Content-Type: application/json", "--data-binary", "@$ReqFile")
  }
  if ($headers) { foreach ($k in $headers.Keys) { $cargs += @("-H", "$k`: $($headers[$k])") } }
  $code = (& curl.exe @cargs | Out-String).Trim()
  $txt = ""
  if (Test-Path $BodyFile) { $txt = Get-Content $BodyFile -Raw -Encoding UTF8 -ErrorAction SilentlyContinue }
  if ($null -eq $txt) { $txt = "" }
  return @{ Status = $code; Content = $txt.Trim() }
}

function EnsureUser($username, $nickname, $password) {
  $r = Req "POST" "$U/register" ('{"username":"' + $username + '","password":"' + $password + '","nickname":"' + $nickname + '"}')
  if ($r.Status -ne "200") { Write-Output ("  注册 " + $username + "：(可能已存在) " + $r.Content) }
  else { Write-Output ("  注册 " + $username + "：OK") }
  $r = Req "POST" "$U/login" ('{"username":"' + $username + '","password":"' + $password + '"}')
  $o = $r.Content | ConvertFrom-Json
  return $o.data
}

Write-Output "=== 1. 创建账号并登录 ==="
$a = EnsureUser "alice" "爱丽丝" "123456"
$b = EnsureUser "bob" "鲍勃" "123456"
$c = EnsureUser "carol" "卡罗尔" "123456"
if (-not $a -or -not $b -or -not $c) { Write-Output "创建账号失败，请确认服务已启动"; exit 1 }

$HA = @{ Authorization = "Bearer $($a.access_token)" }
$HB = @{ Authorization = "Bearer $($b.access_token)" }
$HC = @{ Authorization = "Bearer $($c.access_token)" }
$idA = $a.user.id; $idB = $b.user.id; $idC = $c.user.id
Write-Output "  alice=$idA bob=$idB carol=$idC"

Write-Output "`n=== 2. 加好友（alice ↔ bob / alice ↔ carol）==="
function AddFriend($fromH, $toId) {
  $r = Req "POST" "$U/friends/requests" ('{"to_user_id":' + $toId + ',"message":"你好"}') $fromH
  return $r.Status
}
AddFriend $HA $idB | Out-Null
AddFriend $HA $idC | Out-Null
Start-Sleep -Milliseconds 300

function AcceptAll($token) {
  $r = Req "GET" "$U/friends/requests" $null @{ Authorization = "Bearer $token" }
  $list = ($r.Content | ConvertFrom-Json).data.requests
  foreach ($req in $list) {
    Req "POST" "$U/friends/requests/$($req.id)/accept" $null @{ Authorization = "Bearer $token" } | Out-Null
  }
}
AcceptAll $b.access_token
AcceptAll $c.access_token
$r = Req "GET" "$U/friends" $null $HA
Write-Output ("  alice 好友数：" + (($r.Content | ConvertFrom-Json).data.friends.Count))

Write-Output "`n=== 3. 单聊（alice ↔ bob）==="
$r = Req "POST" "$U/conversations" ('{"peer_user_id":' + $idB + '}') $HA
$convId = ($r.Content | ConvertFrom-Json).data.conversation_id
$msgs = @("你好 bob，这是演示消息", "WebSocket 实时消息演示", "试试点右上角电话图标打语音")
$i = 0
foreach ($m in $msgs) {
  $i++
  Req "POST" "$U/conversations/$convId/messages" ('{"client_msg_id":"seed-a-' + $i + '","type":1,"content":"' + $m + '"}') $HA | Out-Null
}
Req "POST" "$U/conversations/$convId/messages" '{"client_msg_id":"seed-b-1","type":1,"content":"收到！界面挺像微信的"}' $HB | Out-Null
Write-Output "  会话 $convId，消息已发送"

Write-Output "`n=== 4. 建群（三人）==="
$r = Req "POST" "$U/groups" ('{"name":"Go 学习小组","announcement":"一起学 Go！","member_ids":[' + $idB + ',' + $idC + ']}') $HA
$g = $r.Content | ConvertFrom-Json
if ($r.Status -eq "200") {
  $gConv = $g.data.conversation_id
  Req "POST" "$U/conversations/$gConv/messages" '{"client_msg_id":"seed-g-1","type":1,"content":"大家好，这是群聊演示"}' $HA | Out-Null
  Write-Output ("  群已创建：id=" + $g.data.id + " conversation_id=" + $gConv)
} else {
  Write-Output ("  建群：可能已存在 " + $r.Content)
}

Write-Output "`n=== 完成 ==="
Write-Output "演示账号：alice / bob / carol，密码都是 123456"
Write-Output "打开 http://127.0.0.1:8080/ 登录体验"
