!INCLUDE "MUI.nsh"

Name "ZDRCT"
OutFile "dist.exe"
InstallDir "$PROGRAMFILES\zdrct"
RequestExecutionLevel user
Unicode false
XPStyle on
ShowInstDetails show

!insertmacro MUI_PAGE_LICENSE "LICENSE.txt"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"

Section "install"
	SetOutPath "$INSTDIR"

	File "zdrct.exe"
	File "LICENSE.txt"

	SetOutPath "$INSTDIR\templates"
	File /x *.swp "templates\*.html"

	SetOutPath "$INSTDIR\assets"
	File /x *.swp "assets\*.*"

	SetOutPath "$INSTDIR"
	WriteUninstaller "uninst.exe"
SectionEnd

Section "Uninstall"
	RMDir /r "$INSTDIR\templates"
	RMDir /r "$INSTDIR\assets"
	Delete "$INSTDIR\zdrct.exe"
	Delete "$INSTDIR\uninst.exe"
	Delete "$SMPROGRAMS\ZDRCT.lnk"
	RMDir "$INSTDIR"
SectionEnd

Section "Start Menu Shortcuts"
	SetRegView 64
	CreateShortCut "$SMPROGRAMS\ZDRCT.lnk" "$INSTDIR\zdrct.exe" "" "$INSTDIR\assets\favicon.ico" 0
SectionEnd

Function done
	MessageBox MB_YESNO "Do you want to start ZDRCT?" IDNO done
	Exec '"$INSTDIR\zdrct.exe"'
done:
	Nop
FunctionEnd

Page custom done "" "Finish"
