; Установщик PICO Trackers для Windows (Inno Setup 7). Собирается в build-windows.sh:
;   ISCC /DAppVersion=… /DSourceDir=<папка программы> /DResDir=<значок и картинки> /DOutputDir=… installer.iss
;
; Языки: английский и русский — по языку Windows (тексты — в [Messages] и [CustomMessages], ru.* и en.*).
; Ставит программу для текущего пользователя (без прав администратора) и по галочке — драйвер SteamVR
; (picotrackers.exe install-driver). Приложение на шлем ставит мастер в самой программе при первом запуске.
; Деинсталлятор снимает драйвер с регистрации в SteamVR и удаляет данные программы (picotrackers.exe uninstall).
;
; Обновление из программы (platform_windows.go, installUpdate): после её выхода — /SILENT /SUPPRESSMSGBOXES
; /NORESTART /UPDATE. Галочки берутся с прошлой установки; драйвер обновляется и тогда, когда его ставил
; мастер в программе, а не установщик; в конце программа запускается снова (picotrackers.exe --updated).

#ifndef AppVersion
  #error Укажите /DAppVersion=…
#endif

[Setup]
AppId={{4C552D11-D2BF-482B-9298-95BD2441EB7F}
AppName=PICO Trackers
AppVersion={#AppVersion}
AppVerName=PICO Trackers {#AppVersion}
AppPublisher=PICOtrackers
AppComments=PICO Motion Trackers in SteamVR over Steam Link
VersionInfoVersion={#AppVersion}
VersionInfoDescription=PICO Trackers Setup
DefaultDirName={autopf}\PICO Trackers
DefaultGroupName=PICO Trackers
DisableProgramGroupPage=yes
DisableWelcomePage=no
PrivilegesRequired=lowest
SetupArchitecture=x64
MinVersion=10.0
; Программа создаёт этот mutex (platform_windows.go, markRunning): установщик и деинсталлятор просят её закрыть.
AppMutex=PICOtrackers.Control
SourceDir={#SourceDir}
OutputDir={#OutputDir}
OutputBaseFilename=PICOtrackers-setup-{#AppVersion}
SetupIconFile={#ResDir}\picotrackers.ico
UninstallDisplayIcon={app}\picotrackers.exe
UninstallDisplayName=PICO Trackers
WizardStyle=modern dynamic windows11
WizardImageFile={#ResDir}\wizard-202.png,{#ResDir}\wizard-269.png,{#ResDir}\wizard-336.png,{#ResDir}\wizard-403.png,{#ResDir}\wizard-498.png
WizardSmallImageFile={#ResDir}\small-58.png,{#ResDir}\small-77.png,{#ResDir}\small-97.png,{#ResDir}\small-116.png,{#ResDir}\small-143.png
Compression=lzma2/ultra64
SolidCompression=yes
ShowLanguageDialog=no

[Languages]
; Первый — для Windows не на русском (Inno выбирает язык по языку интерфейса Windows).
Name: "en"; MessagesFile: "compiler:Default.isl"
Name: "ru"; MessagesFile: "compiler:Languages\Russian.isl"

[Messages]
en.WelcomeLabel2=This will install [name/ver] on your computer.%n%nPICO Trackers brings PICO Motion Trackers into SteamVR when you play over Steam Link.%n%nIt is better to close SteamVR before installing.
en.FinishedLabel=[name] is installed.%n%nOn the first start it will help you install the app on the headset: connect the PICO to the computer with a cable and turn on USB debugging in the headset.
en.FinishedLabelNoIcons=[name] is installed.%n%nOn the first start it will help you install the app on the headset: connect the PICO to the computer with a cable and turn on USB debugging in the headset.
ru.WelcomeLabel2=Программа установит [name/ver] на ваш компьютер.%n%nPICO Trackers передаёт PICO Motion Trackers в SteamVR, когда вы играете через Steam Link.%n%nПеред установкой лучше закрыть SteamVR.
ru.FinishedLabel=Программа [name] установлена.%n%nПри первом запуске она поможет поставить приложение на шлем: подключите PICO к компьютеру кабелем и включите в шлеме отладку по USB.
ru.FinishedLabelNoIcons=Программа [name] установлена.%n%nПри первом запуске она поможет поставить приложение на шлем: подключите PICO к компьютеру кабелем и включите в шлеме отладку по USB.

[CustomMessages]
en.TaskDriver=Install the SteamVR driver (trackers appear in SteamVR)
en.TaskAutostart=Start PICO Trackers when you sign in to Windows
en.GroupAdditional=Additional:
en.InstallingDriver=Installing the SteamVR driver…
en.DriverInstalled=SteamVR driver installed
en.DriverNotInstalled=SteamVR driver not installed
en.SteamVRRunningInstall=SteamVR is running, and Windows does not allow replacing the driver it has loaded. Close SteamVR and press “Retry”.
en.SkipDriverInstall=If you skip, you can install the driver later in the program: menu → “Setup wizard”.
en.RemovingDriver=Removing the SteamVR driver…
en.DriverNotRemoved=SteamVR driver not removed
en.SteamVRRunningUninstall=SteamVR is running and holds the driver. Close SteamVR and press “Retry”.
en.SkipDriverRemove=If you skip, the driver stays registered in SteamVR.
en.ErrorCode=Error %1.
en.Retry=Retry
en.Skip=Skip
ru.TaskDriver=Установить драйвер SteamVR (трекеры появятся в SteamVR)
ru.TaskAutostart=Запускать PICO Trackers при входе в Windows
ru.GroupAdditional=Дополнительно:
ru.InstallingDriver=Установка драйвера SteamVR…
ru.DriverInstalled=Драйвер SteamVR установлен
ru.DriverNotInstalled=Драйвер SteamVR не установлен
ru.SteamVRRunningInstall=SteamVR запущен, а Windows не даёт заменить загруженный им драйвер. Закройте SteamVR и нажмите «Повторить».
ru.SkipDriverInstall=Если пропустить, драйвер можно будет установить позже в программе: меню → «Мастер установки».
ru.RemovingDriver=Удаление драйвера SteamVR…
ru.DriverNotRemoved=Драйвер SteamVR не удалён
ru.SteamVRRunningUninstall=SteamVR запущен и держит драйвер. Закройте SteamVR и нажмите «Повторить».
ru.SkipDriverRemove=Если пропустить, драйвер останется зарегистрированным в SteamVR.
ru.ErrorCode=Ошибка %1.
ru.Retry=Повторить
ru.Skip=Пропустить

[Tasks]
Name: "driver"; Description: "{cm:TaskDriver}"
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked
Name: "autostart"; Description: "{cm:TaskAutostart}"; GroupDescription: "{cm:GroupAdditional}"; Flags: unchecked

[InstallDelete]
; DLL от прошлой версии: у GTK при обновлении меняются имена библиотек.
Type: files; Name: "{app}\*.dll"

[Files]
Source: "*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{autoprograms}\PICO Trackers"; Filename: "{app}\picotrackers.exe"
Name: "{autodesktop}\PICO Trackers"; Filename: "{app}\picotrackers.exe"; Tasks: desktopicon

[Registry]
; То же значение, что пишет переключатель «Автозапуск» в программе (platform_windows.go, setAutostart).
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "PICO Trackers"; ValueData: """{app}\picotrackers.exe"""; Flags: uninsdeletevalue; Tasks: autostart

[Run]
Filename: "{app}\picotrackers.exe"; Description: "{cm:LaunchProgram,PICO Trackers}"; Flags: nowait postinstall skipifsilent

[Code]
const
  SteamVRRunning = 3; { код выхода picotrackers.exe: мешает запущенный SteamVR }

{ Запускает picotrackers.exe с командой для установщика. Message — сообщение программы для пользователя. }
function RunProgram(const Command: String; var Message: String): Integer;
var
  MessageFile: String;
  Text: AnsiString;
begin
  MessageFile := ExpandConstant('{tmp}\picotrackers-message.txt');
  DeleteFile(MessageFile);
  Message := '';
  if not Exec(ExpandConstant('{app}\picotrackers.exe'), Command + ' "--message-file=' + MessageFile + '"', '',
              SW_HIDE, ewWaitUntilTerminated, Result) then
  begin
    Message := SysErrorMessage(Result);
    Result := -1;
    Exit;
  end;
  { Файл в UTF-8 и уже готов к показу: Trim, Copy и AnsiUppercase в Pascal Script Inno Setup 7 работают
    через ANSI и на нерусской Windows превращают кириллицу в «?» (проверено), поэтому их здесь нет. }
  if LoadStringFromFile(MessageFile, Text) then
    Message := Utf8Decode(Text);
  Log('picotrackers.exe ' + Command + ': код ' + IntToStr(Result) + ', ' + Message);
end;

{ Повторять ли действие: окно с кнопками «Повторить» / «Пропустить». При /SUPPRESSMSGBOXES — «Пропустить». }
function AskRetry(const Instruction, Text: String): Boolean;
begin
  { Строка не должна начинаться с «[» — Inno примет её за заголовок секции. }
  Result := SuppressibleTaskDialogMsgBox(Instruction, Text, mbError, MB_RETRYCANCEL, [CustomMessage('Retry'),
    CustomMessage('Skip')], 0, IDCANCEL) = IDRETRY;
end;

{ Запущен программой для обновления (параметр /UPDATE). }
function IsUpdate: Boolean;
var
  I: Integer;
begin
  Result := False;
  for I := 1 to ParamCount do
    if CompareText(ParamStr(I), '/UPDATE') = 0 then
      Result := True;
end;

{ Драйвер, установленный программой (picotrackers.exe install-driver, мастер): core.go, driverInstallDir. }
function DriverInstalled: Boolean;
begin
  Result := DirExists(ExpandConstant('{localappdata}\PICOtrackers\driver\picotrackers'));
end;

procedure InstallDriver;
var
  Code: Integer;
  Message: String;
begin
  WizardForm.StatusLabel.Caption := CustomMessage('InstallingDriver');
  repeat
    Code := RunProgram('install-driver', Message);
    if Code = 0 then
    begin
      if Message <> '' then
        SuppressibleTaskDialogMsgBox(CustomMessage('DriverInstalled'), Message, mbInformation, MB_OK, [], 0, IDOK);
      Exit;
    end;
    if Code = SteamVRRunning then
      Message := CustomMessage('SteamVRRunningInstall')
    else if Message = '' then
      Message := FmtMessage(CustomMessage('ErrorCode'), [IntToStr(Code)]);
  until not AskRetry(CustomMessage('DriverNotInstalled'), Message + #13#10#13#10 + CustomMessage('SkipDriverInstall'));
end;

procedure CurStepChanged(CurStep: TSetupStep);
var
  Code: Integer;
begin
  if (CurStep = ssPostInstall) and (WizardIsTaskSelected('driver') or (IsUpdate and DriverInstalled)) then
    InstallDriver;
  if (CurStep = ssDone) and IsUpdate then
    Exec(ExpandConstant('{app}\picotrackers.exe'), '--updated', '', SW_SHOWNORMAL, ewNoWait, Code);
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  Code: Integer;
  Message: String;
begin
  if CurUninstallStep <> usUninstall then
    Exit;
  UninstallProgressForm.StatusLabel.Caption := CustomMessage('RemovingDriver');
  repeat
    Code := RunProgram('uninstall', Message);
    if Code = 0 then
      Exit;
    if Code = SteamVRRunning then
      Message := CustomMessage('SteamVRRunningUninstall')
    else if Message = '' then
      Message := FmtMessage(CustomMessage('ErrorCode'), [IntToStr(Code)]);
  until not AskRetry(CustomMessage('DriverNotRemoved'), Message + #13#10#13#10 + CustomMessage('SkipDriverRemove'));
end;
