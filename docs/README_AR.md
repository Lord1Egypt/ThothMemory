# ThothMemory — ذاكرة مشتركة لمساعدي البرمجة

يحفظ ThothMemory أفكار المشروع وملاحظاته وقراراته ومهامه في قاعدة بيانات
محلية. يمكنك إغلاق مساعد وفتح آخر ليقرأ ما تم حفظه ويكمل العمل.
يدعم الاتصال عبر MCP مع Claude Code وCodex وOpenCode.

## أي نسخة أختار؟

| النسخة | مناسبة لمن؟ | المتطلبات |
|---|---|---|
| **UltraLite** | أغلب المستخدمين على الكمبيوتر | ملف تنفيذي واحد وSQLite؛ دون Docker |
| Full | من يريد لوحة متصفح وخدمة مستمرة | Docker وPostgreSQL |
| Mini | مستخدمو ThothDock والبيئات الصغيرة | محرك متوافق مع Docker؛ MariaDB داخل الحاوية |

ابدأ بـUltraLite. لا تحتاج إلى تثبيت Go أو قاعدة بيانات منفصلة.
الإصدار الجامع [v0.2.3](https://github.com/Lord1Egypt/ThothMemory/releases/tag/v0.2.3)
يجمع ملفات Full وUltraLite وروابط صور Mini في صفحة واحدة.
الإصدار القديم v0.1.2 يحتوي Full فقط.

**تنبيه عند استخدام Full:** ثبّته دائمًا في مجلد جديد فارغ؛ فالأمر `./setup.sh` ينشئ وحدة تخزين خاصة
بالتثبيت الجديد ولا يعيد استخدام وحدة موجودة أو يرقّيها. لترقية تثبيت Full قائم خذ نسخة احتياطية أولًا ثم اتبع
`docs/UPGRADE.md` (الأمر `adopt-volume`). الاستعادة من نسخة احتياطية تتم دائمًا في قاعدة بيانات جديدة.

## Windows

من v0.2.3، نزّل `thothmemory-ultralite-windows-amd64.exe` و`SHA256SUMS`.
افتح PowerShell في مجلد التنزيل. قارن ناتج الأمر الأول بالقيمة في ملف البصمات:

```powershell
Get-FileHash .\thothmemory-ultralite-windows-amd64.exe -Algorithm SHA256
$bin = "$env:LOCALAPPDATA\Programs\ThothMemory"
New-Item -ItemType Directory -Force $bin | Out-Null
Copy-Item .\thothmemory-ultralite-windows-amd64.exe "$bin\thothmemory-ultralite.exe"
& "$bin\thothmemory-ultralite.exe" init
& "$bin\thothmemory-ultralite.exe" doctor
```

انتقل إلى مجلد مشروعك. غيّر المسار التالي إلى مسار حقيقي:

```powershell
Set-Location C:\path\to\your-project
& "$bin\thothmemory-ultralite.exe" connect all
& "$bin\thothmemory-ultralite.exe" instructions
```

لا تحتاج إلى صلاحيات مسؤول. احتفظ بالملف في مكانه لأن المساعد يشغله بمساره
الكامل. خطافات الأوامر غير مدعومة على Windows الأصلي؛ استخدم تعليمات المشروع
وكتابات MCP، أو WSL عند الحاجة إلى خطافات مدعومة.

## Linux

نزّل `thothmemory-ultralite-linux-amd64`، أو ملف `linux-arm64` لجهاز ARM64.
من مجلد التنزيل:

```sh
sha256sum thothmemory-ultralite-linux-amd64
mkdir -p ~/.local/bin
install -m 0755 thothmemory-ultralite-linux-amd64 ~/.local/bin/thothmemory-ultralite
~/.local/bin/thothmemory-ultralite init
cd /path/to/your-project
~/.local/bin/thothmemory-ultralite connect all
~/.local/bin/thothmemory-ultralite instructions
~/.local/bin/thothmemory-ultralite doctor
```

استبدل اسم الملف بـ`thothmemory-ultralite-linux-arm64` في أمر التثبيت على ARM.
المسار الكامل يغنيك عن إعداد PATH. يجب أن يكون للمشروع Git remote باسم `origin`
حتى يتعرف عليه المساعد تلقائيًا.

## توصيل المساعد والذاكرة التلقائية

ثبّت CLI للمساعد الذي تختاره أولًا. يسجّل `connect all` المساعدين المثبتين.
يمكن اختيار واحد عبر `connect claude` أو `connect codex` أو `connect opencode`.
التسجيل يخص المشروع الحالي؛ أضف `--scope user` ليشمل جميع المشاريع.
يعرض البرنامج التغييرات ويطلب الموافقة، ويحافظ على الإعدادات والنصوص الأخرى.

أعد تشغيل المساعد ووافق على طلب الثقة العادي، ثم افحص `claude mcp list`
أو `codex mcp list`. وجود الأدوات لا يعني حفظ كل المحادثة تلقائيًا:
`instructions` يعلّم النموذج تسجيل المعلومات المهمة، والخطافات تسجل أحداث
الجلسة المدعومة. على Linux أو WSL مع Claude:

```sh
~/.local/bin/thothmemory-ultralite hooks claude
~/.local/bin/thothmemory-ultralite capture status
~/.local/bin/thothmemory-ultralite capture set metadata
```

كتابات Codex عبر MCP ونقل الذاكرة منه إلى Claude مجرّبان. التقاط خطافات Codex
لم يُتحقق منه؛ يحتاج أيضًا إلى ثقة المجلد ومراجعة `/hooks`. اتصال OpenCode
مجرّب، لكن جلسة نموذج حقيقية لم تُختبر. حفظ طلبات المستخدم اختياري عبر
`capture set prompts --yes`؛ لا تُجمع المحادثات الكاملة. للإيقاف استخدم
`capture set off`.

## جرّب أول فكرة

قل للمساعد: «قد نضيف SQLite إلى PocketClaw. سجّلها كفكرة مقترحة وليست قرارًا».
أغلقه وافتح مساعدًا ثانيًا في المشروع نفسه، ثم اسأل عن الأفكار المسجلة.
تحقق من قاعدة البيانات بنفسك، باستخدام معرّف المشروع الناتج من الأمر الأول:

```sh
~/.local/bin/thothmemory-ultralite projects
~/.local/bin/thothmemory-ultralite search PROJECT_ID PocketClaw
```

إذا قال المساعد «حفظت» دون نتيجة فعلية، اطلب تنفيذ كتابة MCP وإظهار أي خطأ.

## مكان البيانات والنسخ الاحتياطي

Windows: `%LOCALAPPDATA%\ThothMemory`. Linux: `~/.local/share/thothmemory`
ما لم تغيّر XDG_DATA_HOME أو تستخدم `--data DIR`.

```sh
~/.local/bin/thothmemory-ultralite backup
~/.local/bin/thothmemory-ultralite restore /path/to/backup.db --confirm
```

احتفظ بملف النسخة وملف `.sha256` في مكان مستقل. أغلق المساعدين قبل الاستعادة.
على Windows استخدم مسار البرنامج الكامل كما في الأمثلة السابقة. للتحديث،
استبدل الملف التنفيذي بعد النسخ الاحتياطي واترك مجلد البيانات كما هو.

## عند حدوث مشكلة

شغّل `doctor` وتأكد من مسار البرنامج ومعمارية الجهاز وثقة المشروع. إذا لم
يظهر المساعد ضمن `connect`، ثبّت CLI وأعد الأمر. افحص الأحداث المؤجلة عبر
`capture pending` وأعد المحاولة بـ`capture retry`. لا تحذف قاعدة البيانات لحل
مشكلة اتصال. لا تحفظ كلمات مرور أو مفاتيح خاصة؛ فحص الأسرار ليس ضمانًا كاملًا.
البيانات محلية، لكن المساعد السحابي قد يرسل السياق المسترجع إلى مزود النموذج.

[الدليل الكامل](GETTING_STARTED.md) · [Mini وAndroid](MINI.md) ·
[الأمان](SECURITY.md) · [النسخ والاستعادة](UPGRADE.md).
