#define MyAppName "打工搭子 WorkMate"
#define MyAppVersion "0.8.0"
#define MyAppPublisher "WorkMate"
#define MyAppExeName "WorkMate.exe"

[Setup]
AppId={{A9927504-6242-483E-A898-517291FA8CFB}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppVerName={#MyAppName} {#MyAppVersion}
AppPublisher={#MyAppPublisher}
DefaultDirName={localappdata}\Programs\WorkMate
DefaultGroupName={#MyAppName}
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
OutputDir=..\dist
OutputBaseFilename=WorkMate-Setup-{#MyAppVersion}
SetupIconFile=..\WorkMate.ico
UninstallDisplayIcon={app}\WorkMate.ico
Compression=lzma2/ultra64
SolidCompression=yes
WizardStyle=modern
CloseApplications=yes
CloseApplicationsFilter=WorkMate.exe
RestartApplications=no
UsePreviousAppDir=yes
AppMutex=Local\WorkMate.SingleInstance,Local\WorkMateV7NativeSingleInstance
SetupMutex=WorkMate.Setup.SingleInstance
SetupLogging=yes
AllowNoIcons=yes
VersionInfoVersion=0.8.0
VersionInfoCompany={#MyAppPublisher}
VersionInfoDescription={#MyAppName} 安装程序
VersionInfoProductName={#MyAppName}
VersionInfoProductVersion={#MyAppVersion}
VersionInfoTextVersion={#MyAppVersion}
MinVersion=10.0

[Tasks]
Name: "desktopicon"; Description: "创建桌面快捷方式"; GroupDescription: "附加选项："; Flags: unchecked

[Files]
Source: "..\dist\WorkMate.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\WorkMate.ico"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\README.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\LEGAL-NOTES.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\assets\payment_qr.png"; DestDir: "{app}"; DestName: "payment_qr.png"; Flags: ignoreversion

Source: "..\assets\commerce.json"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\WorkMate"; Filename: "{app}\WorkMate.exe"; WorkingDir: "{app}"; IconFilename: "{app}\WorkMate.ico"
Name: "{autodesktop}\WorkMate"; Filename: "{app}\WorkMate.exe"; WorkingDir: "{app}"; IconFilename: "{app}\WorkMate.ico"; Tasks: desktopicon

[Run]
Filename: "{app}\WorkMate.exe"; Description: "启动 WorkMate"; Flags: nowait postinstall skipifsilent

[UninstallDelete]
Type: filesandordirs; Name: "{app}"
