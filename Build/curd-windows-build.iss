[Setup]
AppName=Curd Installer
AppVersion=1.0.8
DefaultDirName={userappdata}\Curd
PrivilegesRequired=lowest
AllowNoIcons=yes
OutputBaseFilename=curd-windows-installer
UsePreviousAppDir=yes
Compression=lzma2
SolidCompression=yes

[Tasks]
; Define a task for creating a desktop shortcut
Name: "desktopicon"; Description: "Create a &desktop shortcut"; GroupDescription: "Additional Options";

[Files]
; Copy the Curd executable to the install directory
Source: "..\releases\curd-{#SetupSetting("AppVersion")}\windows\curd-windows-x86_64.exe"; DestDir: "{app}"; DestName: "curd.exe"; Flags: ignoreversion
Source: "mpv\mpv.exe"; DestDir: "{app}\bin"; Flags: ignoreversion

[Icons]
; Create the application icon in the Start Menu
Name: "{group}\Curd"; Filename: "{app}\curd.exe"
; Create a desktop shortcut if the user checked the option
Name: "{userdesktop}\Curd"; Filename: "{app}\curd.exe"; Tasks: desktopicon
