; TallyBridge Connector — Inno Setup Script
; Build: iscc build\installer\tallybridge-setup.iss
; Requires Inno Setup 6.x  (https://jrsoftware.org/isdownload.php)
;
; Environment variables (optional — set before calling iscc):
;   APP_VERSION   e.g. 1.2.0   (defaults to 1.0.0)
;   ARCH          x64 | x86 | arm64 | all   (defaults to x64)
;
; The script auto-selects the correct executable based on ARCH.
; For multi-arch distribution, build separate installers per arch.

#ifndef AppVersion
  #define AppVersion GetEnv("APP_VERSION")
  #if AppVersion == ""
    #define AppVersion "1.0.0"
  #endif
#endif

#ifndef Arch
  #define Arch GetEnv("ARCH")
  #if Arch == ""
    #define Arch "x64"
  #endif
#endif

#if Arch == "arm64"
  #define ExeName "tallybridge-connector-arm64.exe"
  #define ArchSuffix "-arm64"
  #define ArchMode "arm64"
#elif Arch == "x86"
  #define ExeName "tallybridge-connector-x86.exe"
  #define ArchSuffix "-x86"
  #define ArchMode "x86"
#else
  ; Default: x64
  #define ExeName "tallybridge-connector-x64.exe"
  #define ArchSuffix ""
  #define ArchMode "x64compatible"
#endif

[Setup]
AppId={{8B2F3C1A-4E5D-4F6A-9B7C-2D3E4F5A6B7C}
AppName=TallyBridge Connector
AppVersion={#AppVersion}
AppPublisher=OneBook
AppPublisherURL=https://onebook.app
AppSupportURL=https://onebook.app/support
AppUpdatesURL=https://onebook.app/updates
AppCopyright=Copyright (C) 2024-2026 OneBook

; Install to Program Files
DefaultDirName={autopf}\TallyBridge
DefaultGroupName=TallyBridge
DisableProgramGroupPage=yes

; Output — relative to the repo root (ISS file is at build\installer\)
OutputDir=..\..\dist
OutputBaseFilename=tallybridge-connector-setup-{#AppVersion}{#ArchSuffix}
; Source files root (repo root)
SourceDir=..\..

; Appearance
SetupIconFile=build\windows\icon.ico
WizardStyle=modern
WizardSizePercent=100
WizardResizable=no

; Compression
Compression=lzma2/ultra64
SolidCompression=yes
LZMAUseSeparateProcess=yes

; Architecture
ArchitecturesAllowed={#ArchMode}
ArchitecturesInstallIn64BitMode={#ArchMode}

; Privileges — install per-user to avoid UAC (matches HKCU auto-start)
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog

; Uninstall
UninstallDisplayIcon={app}\TallyBridgeConnector.exe
UninstallDisplayName=TallyBridge Connector {#AppVersion}
CreateUninstallRegKey=yes

; Misc
ShowLanguageDialog=no
DisableWelcomePage=no
DisableDirPage=no
DisableReadyPage=no
AllowCancelDuringInstall=yes

; Restart — not needed (no kernel drivers)
RestartIfNeededByRun=no

; Minimum OS: Windows 10
MinVersion=10.0

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon";   Description: "Create a &desktop shortcut";          GroupDescription: "Additional icons:"; Flags: unchecked
Name: "startupentry";  Description: "&Start TallyBridge automatically with Windows"; GroupDescription: "Startup:"; Flags: unchecked

[Files]
; Main executable — renamed to a friendly name at destination
Source: "build\bin\{#ExeName}"; DestDir: "{app}"; DestName: "TallyBridgeConnector.exe"; Flags: ignoreversion

[Icons]
; Start Menu
Name: "{group}\TallyBridge Connector"; Filename: "{app}\TallyBridgeConnector.exe"; IconFilename: "{app}\TallyBridgeConnector.exe"
Name: "{group}\Uninstall TallyBridge Connector"; Filename: "{uninstallexe}"

; Desktop (optional task)
Name: "{autodesktop}\TallyBridge Connector"; Filename: "{app}\TallyBridgeConnector.exe"; IconFilename: "{app}\TallyBridgeConnector.exe"; Tasks: desktopicon

[Registry]
; Auto-start at Windows login (matches what the app's own toggle writes)
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "TallyBridgeConnector"; ValueData: """{app}\TallyBridgeConnector.exe"""; Flags: uninsdeletevalue; Tasks: startupentry

[Run]
; Launch after install (don't wait — it's a GUI tray app)
Filename: "{app}\TallyBridgeConnector.exe"; Description: "Launch TallyBridge Connector"; Flags: nowait postinstall skipifsilent shellexec

[UninstallRun]
; Kill any running instance before uninstall
Filename: "taskkill.exe"; Parameters: "/F /IM TallyBridgeConnector.exe"; Flags: runhidden; RunOnceId: "KillTallyBridge"

[UninstallDelete]
; Clean up the SQLite database and logs left by the app
Type: filesandordirs; Name: "{userappdata}\TallyBridge"

[Code]
// ---------------------------------------------------------------------------
// Upgrade: if an older version is already installed, silently uninstall it
// before proceeding so files are cleanly replaced.
// ---------------------------------------------------------------------------
function GetUninstallString(): String;
var
  sUnInstPath: String;
  sUnInstallString: String;
begin
  sUnInstPath := ExpandConstant('Software\Microsoft\Windows\CurrentVersion\Uninstall\{#SetupSetting("AppId")}_is1');
  sUnInstallString := '';
  if not RegQueryStringValue(HKCU, sUnInstPath, 'UninstallString', sUnInstallString) then
    RegQueryStringValue(HKLM, sUnInstPath, 'UninstallString', sUnInstallString);
  Result := sUnInstallString;
end;

function IsUpgrade(): Boolean;
begin
  Result := (GetUninstallString() <> '');
end;

function UnInstallOldVersion(): Integer;
var
  sUnInstallString: String;
  iResultCode: Integer;
begin
  Result := 0;
  sUnInstallString := GetUninstallString();
  if sUnInstallString <> '' then begin
    sUnInstallString := RemoveQuotes(sUnInstallString);
    if Exec(sUnInstallString, '/SILENT /NORESTART /SUPPRESSMSGBOXES', '', SW_HIDE, ewWaitUntilTerminated, iResultCode) then
      Result := iResultCode
    else
      Result := 1;
  end;
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssInstall then begin
    if IsUpgrade() then
      UnInstallOldVersion();
  end;
end;
