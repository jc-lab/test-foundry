<#
 Copyright 2026 JC-Lab
 SPDX-License-Identifier: GPL-2.0-only
#>

# trigger-bsod.ps1
#
# NtRaiseHardError를 호출하여 BSOD (Blue Screen of Death)를 발생시킵니다.
# 관리자 권한 필요.
#
# WARNING: 이 스크립트는 시스템을 즉시 크래시시킵니다.

$signature = @"
using System;
using System.Runtime.InteropServices;

public class CrashHelper
{
    [DllImport("ntdll.dll")]
    public static extern uint RtlAdjustPrivilege(
        int Privilege,
        bool bEnablePrivilege,
        bool IsThreadPrivilege,
        out bool PreviousValue
    );

    [DllImport("ntdll.dll")]
    public static extern uint NtRaiseHardError(
        uint ErrorStatus,
        uint NumberOfParameters,
        uint UnicodeStringParameterMask,
        IntPtr Parameters,
        uint ValidResponseOption,
        out uint Response
    );
}
"@

Add-Type -TypeDefinition $signature

# SeShutdownPrivilege 활성화
$previousValue = $false
[CrashHelper]::RtlAdjustPrivilege(19, $true, $false, [ref]$previousValue)

$response = [uint32]0
[CrashHelper]::NtRaiseHardError([uint32]'0xC0000420', 0, 0, [IntPtr]::Zero, 6, [ref]$response)
