[Setup]
AppName=Curd Installer
AppVersion=0.0.10
DefaultDirName={pf}\Curd
DefaultGroupName=Curd
AllowNoIcons=yes
OutputBaseFilename=CurdInstaller
UsePreviousAppDir=yes
Compression=lzma2
SolidCompression=yes

[Tasks]
; Define a task for creating a desktop shortcut
Name: "desktopicon"; Description: "Create a &desktop shortcut"; GroupDescription: "Additional Options";

[Files]
; Copy the Curd executable to the install directory
Source: "Z:releases/curd-0.0.10/windows/curd.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "Z:/home/wraient/Projects/curd/Build/mpv.exe"; DestDir: "{app}\bin"; Flags: ignoreversion

[Icons]
; Create the application icon in the Start Menu
Name: "{group}\Curd"; Filename: "{app}\curd.exe"
; Create a desktop shortcut if the user checked the option
Name: "{userdesktop}\Curd"; Filename: "{app}\curd.exe"; Tasks: desktopicon
