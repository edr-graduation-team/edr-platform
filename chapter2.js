const {
  Document, Packer, Paragraph, TextRun, Table, TableRow, TableCell,
  AlignmentType, LevelFormat, WidthType, BorderStyle, ShadingType,
  VerticalAlign, HeadingLevel, PageBreak, PageNumber,
  Header, Footer, TableOfContents
} = require('docx');
const fs = require('fs');

// ─── Shared border style for tables ─────────────────────────────────────────
const border = { style: BorderStyle.SINGLE, size: 1, color: "CCCCCC" };
const borders = { top: border, bottom: border, left: border, right: border };
const headerBg = "1F3864"; // dark blue matching academic doc style
const altRowBg = "EBF3FB"; // light blue for alternating rows

// ─── Helper: Arabic paragraph (body text, RTL, justified) ──────────────────
function arParagraph(text, opts = {}) {
  const runs = [];
  if (typeof text === 'string') {
    runs.push(new TextRun({
      text,
      font: "Traditional Arabic",
      size: opts.size || 26,
      bold: opts.bold || false,
      color: opts.color || "000000",
      rtl: true,
    }));
  } else {
    // array of {text, bold?, size?, en?} segments
    for (const seg of text) {
      runs.push(new TextRun({
        text: seg.text,
        font: seg.en ? "Times New Roman" : "Traditional Arabic",
        size: seg.size || opts.size || 26,
        bold: seg.bold !== undefined ? seg.bold : (opts.bold || false),
        color: seg.color || opts.color || "000000",
        rtl: seg.en ? false : true,
      }));
    }
  }
  return new Paragraph({
    bidi: true,
    alignment: opts.align || AlignmentType.RIGHT,
    spacing: { before: opts.before ?? 80, after: opts.after ?? 100 },
    children: runs,
    numbering: opts.numbering,
  });
}

// ─── Helper: English paragraph ──────────────────────────────────────────────
function enParagraph(text, opts = {}) {
  return new Paragraph({
    alignment: opts.align || AlignmentType.LEFT,
    spacing: { before: opts.before ?? 80, after: opts.after ?? 100 },
    children: [new TextRun({
      text,
      font: "Times New Roman",
      size: opts.size || 24,
      bold: opts.bold || false,
      color: opts.color || "000000",
    })],
  });
}

// ─── Helper: Section heading (bold, RTL) ────────────────────────────────────
function sectionHeading(text, opts = {}) {
  const segments = [];
  // Split Arabic / English
  segments.push({ text, bold: true });
  return new Paragraph({
    bidi: true,
    alignment: AlignmentType.RIGHT,
    spacing: { before: opts.before ?? 280, after: opts.after ?? 120 },
    children: [new TextRun({
      text,
      font: opts.en ? "Times New Roman" : "Traditional Arabic",
      size: opts.size || 30,
      bold: true,
      color: opts.color || "000000",
      rtl: !opts.en,
    })],
  });
}

// ─── Helper: Sub-section heading ────────────────────────────────────────────
function subHeading(text, opts = {}) {
  return new Paragraph({
    bidi: true,
    alignment: AlignmentType.RIGHT,
    spacing: { before: opts.before ?? 200, after: opts.after ?? 100 },
    children: [new TextRun({
      text,
      font: "Traditional Arabic",
      size: opts.size || 28,
      bold: true,
      color: opts.color || "1F3864",
      rtl: true,
    })],
  });
}

// ─── Helper: Labeled paragraph (bold label + body) ──────────────────────────
function labelParagraph(label, body, opts = {}) {
  return new Paragraph({
    bidi: true,
    alignment: AlignmentType.RIGHT,
    spacing: { before: opts.before ?? 100, after: opts.after ?? 80 },
    children: [
      new TextRun({ text: label + ": ", font: "Traditional Arabic", size: 26, bold: true, rtl: true, color: "000000" }),
      new TextRun({ text: body, font: "Traditional Arabic", size: 26, bold: false, rtl: true, color: "000000" }),
    ],
  });
}

// ─── Helper: numbered Arabic list item ──────────────────────────────────────
function arBullet(text, opts = {}) {
  const runs = [];
  if (typeof text === 'string') {
    runs.push(new TextRun({ text, font: "Traditional Arabic", size: 26, rtl: true, color: "000000" }));
  } else {
    for (const seg of text) {
      runs.push(new TextRun({
        text: seg.text,
        font: seg.en ? "Times New Roman" : "Traditional Arabic",
        size: 26, rtl: seg.en ? false : true,
        bold: seg.bold || false, color: "000000",
      }));
    }
  }
  return new Paragraph({
    bidi: true,
    alignment: AlignmentType.RIGHT,
    spacing: { before: 60, after: 60 },
    numbering: { reference: "bullets", level: 0 },
    children: runs,
  });
}

// ─── Helper: empty paragraph (spacer) ───────────────────────────────────────
function spacer(before = 60) {
  return new Paragraph({ spacing: { before, after: 0 }, children: [new TextRun("")] });
}

// ─── Helper: table header cell ──────────────────────────────────────────────
function hCell(text, width, en = false) {
  return new TableCell({
    width: { size: width, type: WidthType.DXA },
    borders,
    shading: { fill: headerBg, type: ShadingType.CLEAR },
    margins: { top: 80, bottom: 80, left: 120, right: 120 },
    verticalAlign: VerticalAlign.CENTER,
    children: [new Paragraph({
      bidi: !en,
      alignment: en ? AlignmentType.CENTER : AlignmentType.CENTER,
      children: [new TextRun({
        text, font: en ? "Times New Roman" : "Traditional Arabic",
        size: 24, bold: true, color: "FFFFFF", rtl: !en,
      })],
    })],
  });
}

// ─── Helper: table data cell ─────────────────────────────────────────────────
function dCell(text, width, opts = {}) {
  const bg = opts.bg || "FFFFFF";
  const en = opts.en || false;
  return new TableCell({
    width: { size: width, type: WidthType.DXA },
    borders,
    shading: { fill: bg, type: ShadingType.CLEAR },
    margins: { top: 80, bottom: 80, left: 120, right: 120 },
    verticalAlign: VerticalAlign.CENTER,
    children: [new Paragraph({
      bidi: !en,
      alignment: AlignmentType.CENTER,
      children: [new TextRun({
        text, font: en ? "Times New Roman" : "Traditional Arabic",
        size: 22, bold: opts.bold || false,
        color: opts.color || "000000", rtl: !en,
      })],
    })],
  });
}

// ════════════════════════════════════════════════════════════════════════════
// BUILD DOCUMENT
// ════════════════════════════════════════════════════════════════════════════
const doc = new Document({
  numbering: {
    config: [
      {
        reference: "bullets",
        levels: [{
          level: 0, format: LevelFormat.BULLET, text: "•",
          alignment: AlignmentType.RIGHT,
          style: { paragraph: { bidi: true, indent: { left: 360, hanging: 360 }, spacing: { before: 60, after: 60 } } },
        }],
      },
    ],
  },

  styles: {
    default: { document: { run: { font: "Traditional Arabic", size: 26 } } },
  },

  sections: [{
    properties: {
      page: {
        size: { width: 11906, height: 16838 },
        margin: { top: 1440, right: 1440, bottom: 1440, left: 1440 },
      },
    },

    children: [

      // ══════════════════════════════════════════════════════════════════════
      // CHAPTER HEADER
      // ══════════════════════════════════════════════════════════════════════
      spacer(200),
      new Paragraph({
        bidi: true,
        alignment: AlignmentType.CENTER,
        spacing: { before: 400, after: 240 },
        children: [new TextRun({ text: "الفصل الثاني", font: "Traditional Arabic", size: 36, bold: true, color: "000000", rtl: true })],
      }),
      new Paragraph({
        bidi: true,
        alignment: AlignmentType.CENTER,
        spacing: { before: 200, after: 400 },
        children: [new TextRun({ text: "الخلفية النظرية والدراسات السابقة", font: "Traditional Arabic", size: 36, bold: true, color: "000000", rtl: true })],
      }),
      spacer(120),

      // ══════════════════════════════════════════════════════════════════════
      // 2.1 خلفية الدراسة
      // ══════════════════════════════════════════════════════════════════════
      sectionHeading("2.1  خلفية الدراسة - Background:"),
      spacer(60),

      arParagraph([
        { text: "يندرج هذا المشروع ضمن مجال الأمن السيبراني، وتحديداً في تخصص الكشف والاستجابة على الأجهزة الطرفية " },
        { text: "(Endpoint Detection and Response — EDR)", en: true },
        { text: ". وكما أوضح الفصل الأول في القسم " },
        { text: "(1.1)" , en: true },
        { text: "، باتت الأجهزة الطرفية — من حواسيب مكتبية ومحمولة وخوادم — تُمثّل نقطة الدخول الأكثر استهدافاً في البنى التحتية للمؤسسات، في ظل توسّع نطاق العمل عن بُعد وانتشار البيئات الهجينة. وقد أكّدت الدراسة التطبيقية التي استغرقت قرابة سنة ونصف — والمُوثَّقة في القسم " },
        { text: "(1.3)", en: true },
        { text: " — أن الفجوات الجوهرية في الحلول المفتوحة القائمة تجعل بناء منظومة " },
        { text: "EDR", en: true },
        { text: " مكتملة ضرورةً حقيقية لا رفاهيةً بحثية." },
      ], { after: 120 }),

      arParagraph([
        { text: "صاغ المحلل أنتون تشوفاكين " },
        { text: "(Anton Chuvakin)", en: true },
        { text: " من مؤسسة " },
        { text: "(Gartner)", en: true },
        { text: " مفهوم " },
        { text: "EDR", en: true },
        { text: " عام 2013 [2]، ليُمثّل نقلةً نوعيةً من نهج الحماية الثابت القائم على مطابقة التوقيعات إلى نهج المراقبة الديناميكية المستمرة والتحليل السلوكي والاستجابة الآلية. وتُعرَّف نقاط النهاية بأنها كل جهاز حاسوبي متصل بالشبكة، وتُمثّل الحلقة الأضعف في سلسلة الأمن المؤسسي؛ إذ تشير الإحصاءات إلى أن ما يزيد على 70% من الهجمات الناجحة تبدأ من نقطة نهاية مخترقة [1]." },
      ], { after: 120 }),

      arParagraph([
        { text: "ظلّت حلول مكافحة الفيروسات التقليدية القائمة على مطابقة التوقيعات " },
        { text: "(Signature-Based Detection)", en: true },
        { text: " خط الدفاع الأول لعقود، غير أن هذا النهج بات عاجزاً عن مواجهة التهديدات الحديثة كهجمات البرمجيات الخبيثة عديمة الملفات " },
        { text: "(Fileless Malware)", en: true },
        { text: "، والتهديدات المتقدمة المستمرة " },
        { text: "(APT)", en: true },
        { text: "، وهجمات العيش على الأرض " },
        { text: "(Living-off-the-Land)", en: true },
        { text: " التي تستغل أدوات نظام التشغيل المشروعة كـ" },
        { text: "(PowerShell)", en: true },
        { text: " و" },
        { text: "(WMI)", en: true },
        { text: " لأغراض خبيثة. ومن هنا برزت منظومة " },
        { text: "EDR", en: true },
        { text: " بوصفها الجيل المتقدّم من حلول الحماية، القادر على الجمع بين المراقبة المستمرة واكتشاف التهديدات المعقّدة والاستجابة لها ضمن منظومة موحّدة ومتكاملة [3]." },
      ], { after: 120 }),

      arParagraph([
        { text: "في سياق هذا المشروع، جرى تطوير منصة " },
        { text: "EDR", en: true },
        { text: " مكتملة تستهدف بيئة " },
        { text: "(Microsoft Windows)", en: true },
        { text: "، وتقوم على المعمارية الموزعة متعددة الطبقات الموصوفة في القسم " },
        { text: "(1.2)", en: true },
        { text: ". يجمع البرنامج الطرفي الأحداث عبر جلسة " },
        { text: "(EDRKernelTrace)", en: true },
        { text: " التي تستخدم آلية " },
        { text: "(Event Tracing for Windows — ETW)", en: true },
        { text: " على مستوى نواة نظام التشغيل، لترصد عشرة أنواع من الأحداث: العمليات، والملفات، والشبكة، والسجل، و" },
        { text: "DNS", en: true },
        { text: "، و" },
        { text: "WMI", en: true },
        { text: "، والأنابيب المسمّاة، وتحميل الصور، والأجهزة القابلة للإزالة، وفحص الثغرات. وتُحال هذه الأحداث عبر " },
        { text: "(gRPC)", en: true },
        { text: " إلى مدير الاتصال الذي يمرّرها إلى موضوع " },
        { text: "(events-raw)", en: true },
        { text: " في " },
        { text: "(Apache Kafka)", en: true },
        { text: " ذي الستة أقسام، حيث يستهلكها محرك الكشف للمطابقة مع قواعد " },
        { text: "(Sigma)", en: true },
        { text: " في الوقت الفعلي." },
      ], { after: 120 }),

      arParagraph([
        { text: "يتعامل نظام " },
        { text: "EDR", en: true },
        { text: " مع ثلاث فئات رئيسة من المستخدمين: المحللون الأمنيون المسؤولون عن تحقيق الحوادث ومراجعة التنبيهات، والمسؤولون عن الاستجابة للحوادث المكلّفون باتخاذ إجراءات الاحتواء والمعالجة، وإداريو الأنظمة المعنيون بإدارة نقاط النهاية ومتابعة حالتها. ويُضاف إليهم في السياق الأكاديمي الباحثون المهتمون بتطوير قواعد الكشف ودراسة أنماط التهديدات." },
      ], { after: 160 }),

      // ══════════════════════════════════════════════════════════════════════
      // 2.2 الدراسات السابقة
      // ══════════════════════════════════════════════════════════════════════
      sectionHeading("2.2  الدراسات السابقة - Literature Review:"),
      spacer(60),

      arParagraph([
        { text: "تتوفر في مجال أمن نقاط النهاية عدة حلول مفتوحة المصدر تسعى إلى تقديم بدائل عن المنتجات التجارية المغلقة كـ" },
        { text: "(CrowdStrike Falcon)", en: true },
        { text: " و" },
        { text: "(SentinelOne)", en: true },
        { text: ". وعلى الرغم من أن هذه الحلول تقدّم مستويات متفاوتة من قدرات الرصد والتحليل، فإنها تعاني من قيود جوهرية تحدّ من فاعليتها. يتناول هذا القسم مراجعةً تحليليةً لأبرز هذه الحلول — " },
        { text: "Wazuh", en: true },
        { text: " و" },
        { text: "OpenEDR", en: true },
        { text: " و" },
        { text: "Velociraptor", en: true },
        { text: " و" },
        { text: "OSSEC", en: true },
        { text: " — مع الإشارة إلى أن القسم " },
        { text: "(1.3)", en: true },
        { text: " من الفصل الأول تناول " },
        { text: "(OpenEDR)", en: true },
        { text: " و" },
        { text: "(Wazuh)", en: true },
        { text: " بعمق أكبر بوصفهما المحور الأساسي لدراسة المشكلة." },
      ], { after: 140 }),

      // 2.2.1 Wazuh
      subHeading("2.2.1  Wazuh"),
      spacer(40),

      arParagraph([
        { text: "تُعدّ " },
        { text: "(Wazuh)", en: true },
        { text: " من أوسع الحلول مفتوحة المصدر انتشاراً، إذ تجمع بين كشف التسلل " },
        { text: "(IDS)", en: true },
        { text: " ومراقبة سلامة الملفات " },
        { text: "(FIM)", en: true },
        { text: " وتقييم الامتثال والاستجابة للحوادث، وتعتمد على بنية وكيل-خادم وتتكامل مع " },
        { text: "(Elastic Stack)", en: true },
        { text: " [3]. غير أن التصنيف الأدق لها يضعها في فئة إدارة المعلومات الأمنية " },
        { text: "(SIEM)", en: true },
        { text: " لا في فئة " },
        { text: "EDR", en: true },
        { text: " بمعناها الدقيق، وذلك لقصور جوهري في قدرات المراقبة السلوكية على مستوى النواة والاستجابة الذاتية المستقلة. وقد خضعت المنظومة لتقييم عملي مستفيض موثّق في القسم " },
        { text: "(1.3)", en: true },
        { text: " من الفصل الأول، وأسفر عن رصد جملة من الإشكاليات الجوهرية:" },
      ], { after: 80 }),

      arBullet([
        { text: "الاعتماد على تحليل السجلات: " , bold: true },
        { text: "لا تستفيد من آلية " },
        { text: "(ETW)", en: true },
        { text: " لمراقبة الأحداث على مستوى النواة، مما يُضعف قدرتها على رصد هجمات " },
        { text: "(Fileless Malware)", en: true },
        { text: " واستغلال الأدوات الشرعية." },
      ]),
      arBullet([
        { text: "استهلاك مرتفع للموارد: ", bold: true },
        { text: "تتطلب بنية " },
        { text: "(Elasticsearch + Kibana + Filebeat)", en: true },
        { text: " رفعَ متطلبات الذاكرة والمعالجة والتخزين بصورة ملحوظة، مما يُقيّد نشرها في البيئات المحدودة الموارد." },
      ]),
      arBullet([
        { text: "غياب دعم أصيل لقواعد Sigma: ", bold: true },
        { text: "تعتمد صيغة " },
        { text: "(XML)", en: true },
        { text: " خاصة تختلف عن معيار " },
        { text: "(Sigma)", en: true },
        { text: " المجتمعي، مما يحول دون الاستفادة من المستودعات المجتمعية لقواعد الكشف المحدَّثة باستمرار." },
      ]),
      arBullet([
        { text: "محدودية الاستجابة الذاتية: ", bold: true },
        { text: "آليات الاستجابة النشطة محدودة ولا تدعم استجابةً مستقلةً على الجهاز عند انقطاع الاتصال بالخادم المركزي." },
      ]),
      arBullet([
        { text: "غياب تتبع سلسلة أسلاف العمليات: ", bold: true },
        { text: "لا تُوفّر المنصة آليةً مدمجةً لإعادة بناء شجرة العمليات ورسم سياق الحادثة الأمنية الكاملة." },
      ]),
      spacer(100),

      // 2.2.2 OpenEDR
      subHeading("2.2.2  OpenEDR"),
      spacer(40),

      arParagraph([
        { text: "(OpenEDR)", en: true },
        { text: " مشروع مفتوح المصدر أطلقته شركة " },
        { text: "(Comodo)", en: true },
        { text: " بهدف توفير حل كشف واستجابة مفتوح يُتيح رؤيةً لنشاط نقاط النهاية على نظام " },
        { text: "(Windows)", en: true },
        { text: " [4]. وقد خضعت المنظومة لدراسة معمّقة شملت مراجعة مستودعها البرمجي على " },
        { text: "(GitHub)", en: true },
        { text: " ومحاولة بناء البرنامج الطرفي من المصدر والاختبار عبر المنصة السحابية، كما هو موثّق تفصيلياً في القسم " },
        { text: "(1.3)", en: true },
        { text: " من الفصل الأول. وكشف هذا الفحص المباشر أن الخادم ولوحة التحكم غير مفتوحَي المصدر ويستلزمان اشتراكاً تجارياً بعد فترة تجريبية لا تتجاوز أربعة عشر يوماً، فضلاً عن:" },
      ], { after: 80 }),

      arBullet([
        { text: "ركود تطويري ومشكلات بناء: ", bold: true },
        { text: "آخر تحديث يعود لنحو سنتين، مع إشكاليات متكررة في بناء المشروع من المصدر نظراً لاعتماده على إصدارات موقوفة من المكتبات البرمجية لا تتوافق مع البيئات الحديثة." },
      ]),
      arBullet([
        { text: "غياب محرك كشف سلوكي مستقل: ", bold: true },
        { text: "لا يتضمن محرك كشف يعتمد على قواعد " },
        { text: "(Sigma)", en: true },
        { text: " في الزمن الحقيقي، إذ يُحال تحليل الأحداث بالكامل إلى المنصة السحابية التجارية." },
      ]),
      arBullet([
        { text: "انعدام آليات الاستجابة الفعّالة: ", bold: true },
        { text: "يقتصر على إصدار التنبيهات دون أي إجراء آلي مصاحب، وتغيب آليات الاستجابة كلياً عن نواة المنظومة المفتوحة." },
      ]),
      arBullet([
        { text: "تعقيد بيئة النشر: ", bold: true },
        { text: "يستلزم التشغيل تثبيت " },
        { text: "(Sysmon)", en: true },
        { text: " و" },
        { text: "(Filebeat)", en: true },
        { text: " مسبقاً إضافةً إلى إعدادات مكثّفة على المنصة السحابية، مع واجهة تزخر بمصطلحات مرتبطة بمنظومة منتجات الشركة." },
      ]),
      spacer(100),

      // 2.2.3 Velociraptor
      subHeading("2.2.3  Velociraptor"),
      spacer(40),

      arParagraph([
        { text: "(Velociraptor)", en: true },
        { text: " أداة متخصصة في التحقيق الجنائي الرقمي والاستجابة للحوادث " },
        { text: "(DFIR)", en: true },
        { text: "، تتميز باستخدام لغة استعلام خاصة " },
        { text: "(VQL — Velocidex Query Language)", en: true },
        { text: " تُتيح جمع البيانات من نقاط النهاية بمرونة عالية [5]. وعلى الرغم من قدراتها الجنائية الرقمية المتقدمة، فإن توظيفها بوصفها منصة " },
        { text: "EDR", en: true },
        { text: " متكاملة يصطدم بقيود بنيوية جوهرية:" },
      ], { after: 80 }),

      arBullet([
        { text: "التركيز على ما بعد الحادثة: ", bold: true },
        { text: "صُمّم أساساً لجمع الأدلة والتحقيق بعد وقوع الحادثة، لا لمراقبة تدفق الأحداث آنياً ضمن دورة كشف مستمرة." },
      ]),
      arBullet([
        { text: "غياب محرك كشف في الزمن الحقيقي: ", bold: true },
        { text: "لا يتضمن محرك مطابقة يعمل بصورة مستمرة على تدفق أحداث النواة، مما يُخرجه من تعريف " },
        { text: "EDR", en: true },
        { text: " الدقيق." },
      ]),
      arBullet([
        { text: "عدم معيارية لغة VQL: ", bold: true },
        { text: "لغة خاصة غير منتشرة مقارنةً بـ" },
        { text: "(Sigma)", en: true },
        { text: "، مما يرفع حاجز الدخول ويُقيّد الاستفادة من المستودعات المجتمعية." },
      ]),
      arBullet([
        { text: "محدودية الاستجابة الذاتية: ", bold: true },
        { text: "لا يوفر محرك استجابة مستقلاً يعمل عند انقطاع الاتصال بالخادم، ولا يدعم التقييم السياقي للمخاطر مع تصنيف " },
        { text: "MITRE ATT&CK", en: true },
        { text: " في الوقت الفعلي." },
      ]),
      spacer(100),

      // 2.2.4 OSSEC
      subHeading("2.2.4  OSSEC"),
      spacer(40),

      arParagraph([
        { text: "(OSSEC)", en: true },
        { text: " نظام كشف تسلل مُستضاف " },
        { text: "(HIDS)", en: true },
        { text: " مفتوح المصدر من أقدم الحلول وأكثرها نضجاً، يوفر تحليل السجلات ومراقبة سلامة الملفات وكشف الجذور الخفية والاستجابة النشطة [6]. غير أن نضجه الزمني لا يُعوّض عن الفجوات البنيوية التي تجعله قاصراً عن تلبية متطلبات منظومة " },
        { text: "EDR", en: true },
        { text: " حديثة:" },
      ], { after: 80 }),

      arBullet([
        { text: "بنية تقنية قديمة: ", bold: true },
        { text: "لم تواكب التطورات الحديثة في مجال " },
        { text: "EDR", en: true },
        { text: " من حيث معالجة التدفقات ومطابقة القواعد الآنية على تدفقات أحداث النواة." },
      ]),
      arBullet([
        { text: "الاعتماد الحصري على تحليل السجلات: ", bold: true },
        { text: "لا يدعم " },
        { text: "(ETW)", en: true },
        { text: " ولا يوفر مراقبةً سلوكيةً على مستوى نواة نظام التشغيل، مما يُعميه عن فئات كاملة من التهديدات الحديثة." },
      ]),
      arBullet([
        { text: "صيغة قواعد معقدة وخاصة: ", bold: true },
        { text: "تعتمد " },
        { text: "(XML)", en: true },
        { text: " خاصة تختلف عن معيار " },
        { text: "(Sigma)", en: true },
        { text: "، مما يُعقّد دمج قواعد الكشف المجتمعية المتجددة." },
      ]),
      arBullet([
        { text: "غياب واجهة إدارة ويب حديثة: ", bold: true },
        { text: "يحتاج المستخدمون إلى أدوات خارجية أو سطر الأوامر للإدارة، مما يُضعف سهولة التشغيل." },
      ]),
      spacer(100),

      // 2.2.5 مقارنة بصرية
      subHeading("2.2.5  مقارنة جوانب القصور في الحلول المُراجَعة:"),
      spacer(40),

      arParagraph("يُلخّص الجدول (2-1) أبرز المعايير التقنية وتوافرها في كل حل مقارنةً بالمنظومة المُطوَّرة في هذا المشروع، ليُجلّي بصورة مركّزة حجم الفجوات التي دفعت إلى بناء حل جديد متكامل:", { after: 120 }),

      // TABLE 2-1
      new Paragraph({
        bidi: true, alignment: AlignmentType.CENTER,
        spacing: { before: 80, after: 60 },
        children: [new TextRun({ text: "جدول (2-1): مقارنة جوانب القصور في حلول EDR مفتوحة المصدر", font: "Traditional Arabic", size: 24, bold: true, rtl: true, color: "1F3864" })],
      }),
      new Table({
        width: { size: 9026, type: WidthType.DXA },
        columnWidths: [2400, 1050, 1050, 1050, 1050, 2426],
        rows: [
          new TableRow({ children: [
            hCell("المعيار", 2400),
            hCell("Wazuh", 1050, true),
            hCell("OpenEDR", 1050, true),
            hCell("Velociraptor", 1050, true),
            hCell("OSSEC", 1050, true),
            hCell("منظومتنا", 2426),
          ]}),
          ...[
            ["مراقبة ETW على مستوى النواة", "لا", "جزئي", "لا", "لا", "نعم"],
            ["دعم قواعد Sigma أصيل", "لا", "لا", "لا", "لا", "نعم"],
            ["الكشف في الزمن الحقيقي", "جزئي", "لا", "لا", "لا", "نعم"],
            ["استجابة ذاتية مستقلة (Offline)", "لا", "لا", "لا", "لا", "نعم"],
            ["تخزين WAL عند انقطاع الاتصال", "لا", "لا", "لا", "لا", "نعم"],
            ["تتبع سلسلة أسلاف العمليات", "لا", "لا", "نعم", "لا", "نعم"],
            ["تصنيف MITRE ATT&CK تلقائي", "لا", "لا", "لا", "لا", "نعم"],
            ["لوحة إدارة ويب متكاملة", "نعم", "لا", "نعم", "لا", "نعم"],
            ["مفتوح المصدر بالكامل", "نعم", "جزئي", "نعم", "نعم", "نعم"],
            ["نشر بأمر واحد (Docker Compose)", "لا", "لا", "لا", "لا", "نعم"],
          ].map((row, i) => new TableRow({ children: [
            dCell(row[0], 2400, { bg: i % 2 === 0 ? "FFFFFF" : altRowBg }),
            dCell(row[1], 1050, { bg: i % 2 === 0 ? "FFFFFF" : altRowBg, color: row[1] === "لا" ? "CC0000" : row[1] === "جزئي" ? "D46B08" : "1F7A1F", bold: true, en: false }),
            dCell(row[2], 1050, { bg: i % 2 === 0 ? "FFFFFF" : altRowBg, color: row[2] === "لا" ? "CC0000" : row[2] === "جزئي" ? "D46B08" : "1F7A1F", bold: true, en: false }),
            dCell(row[3], 1050, { bg: i % 2 === 0 ? "FFFFFF" : altRowBg, color: row[3] === "لا" ? "CC0000" : row[3] === "جزئي" ? "D46B08" : "1F7A1F", bold: true, en: false }),
            dCell(row[4], 1050, { bg: i % 2 === 0 ? "FFFFFF" : altRowBg, color: row[4] === "لا" ? "CC0000" : row[4] === "جزئي" ? "D46B08" : "1F7A1F", bold: true, en: false }),
            dCell(row[5], 2426, { bg: i % 2 === 0 ? "E8F5E9" : "C8E6C9", color: "1F7A1F", bold: true }),
          ]})),
        ],
      }),
      spacer(80),
      arParagraph("يتضح من الجدول (2-1) أن المنظومة المُطوَّرة هي الوحيدة التي تحقّق جميع المعايير معاً، بينما لا يُوفّر أيٌّ من الحلول الأخرى مجموعة المعايير كاملةً في آنٍ واحد. ويُشكّل هذا التحليل المقارن الأساس التقني لمسوّغ بناء المنظومة المقترحة في هذا المشروع.", { before: 80, after: 160 }),

      // 2.2.6 الفجوة البحثية
      subHeading("2.2.6  الفجوة البحثية والتطبيقية:"),
      spacer(40),

      arParagraph("يتضح من التحليل السابق أن الحلول مفتوحة المصدر القائمة تعاني من فجوات مشتركة لم يعالجها أيٌّ منها بصورة شاملة. وتتمحور هذه الفجوات في المحاور الآتية:", { after: 80 }),

      arBullet([
        { text: "غياب المراقبة على مستوى نواة النظام: ", bold: true },
        { text: "تعتمد معظم الحلول على السجلات النصية ولا تستفيد من " },
        { text: "(ETW)", en: true },
        { text: "، مما يُعميها عن هجمات الذاكرة وتقنيات التهرب الحديثة." },
      ]),
      arBullet([
        { text: "عدم دعم معيار Sigma بشكل أصيل: ", bold: true },
        { text: "لا يتوفر في أيٍّ من الحلول محرك مطابقة يعمل في الزمن الحقيقي على تدفق أحداث " },
        { text: "ETW", en: true },
        { text: " بصورة مباشرة ومدمجة." },
      ]),
      arBullet([
        { text: "محدودية الاستجابة الذاتية المستقلة: ", bold: true },
        { text: "تفتقر هذه الحلول إلى آلية استجابة تعمل مستقلةً عن الخادم عند انقطاع الاتصال، وهو سيناريو شائع في الهجمات المتقدمة." },
      ]),
      arBullet([
        { text: "ضعف التقييم السياقي للمخاطر: ", bold: true },
        { text: "لا يتوفر نظام تقييم يأخذ في الاعتبار سلسلة أسلاف العملية والسلوك التاريخي لتحديد الخطورة ديناميكياً." },
      ]),
      arBullet([
        { text: "الانغلاق الجزئي أو ضخامة البنية التحتية: ", bold: true },
        { text: "بعض الحلول تشترط ترخيصاً مدفوعاً أو بنيةً تحتيةً ضخمة تُعيق النشر البسيط المحلي." },
      ]),

      spacer(80),
      arParagraph([
        { text: "يسعى المشروع الحالي إلى سدّ هذه الفجوات بمنصة متكاملة مفتوحة المصدر في جميع مكوّناتها الخمسة — البرنامج الطرفي ومدير الاتصال ومحرك الكشف ومحرك الاستجابة ولوحة التحكم — مع دعم ناقل رسائل موزع وقاعدة بيانات مركزية وتصنيف " },
        { text: "MITRE ATT&CK", en: true },
        { text: " آلي في الوقت الفعلي، وذلك ضمن بيئة مفتوحة المصدر بالكامل قابلة للنشر بأمر واحد." },
      ], { after: 160 }),

      // ══════════════════════════════════════════════════════════════════════
      // 2.3 النظام الحالي
      // ══════════════════════════════════════════════════════════════════════
      sectionHeading("2.3  النظام الحالي - Existing System:"),
      spacer(60),

      arParagraph("يتناول هذا القسم وصف الوضع القائم الذي أفضى إلى الحاجة لتطوير المنصة المقترحة، من خلال استعراض المقاربة الدفاعية التقليدية التي كانت سائدة في المؤسسات المتوسطة والبيئات البحثية قبل اعتماد منظومات متكاملة كهذه.", { after: 120 }),

      // 2.3.1
      subHeading("2.3.1  وصف الوضع القائم:"),
      spacer(40),

      arParagraph([
        { text: "كانت المؤسسات المتوسطة والبيئات البحثية تعتمد مقاربةً دفاعيةً مجزّأةً " },
        { text: "(Fragmented Defense Approach)", en: true },
        { text: " تتشكّل في الغالب من ثلاثة عناصر متفرقة لا تُشكّل منظومةً أمنية متكاملة:" },
      ], { after: 80 }),

      labelParagraph("أولاً — برامج مكافحة الفيروسات التقليدية", "المثبّتة مستقلةً على كل نقطة نهاية، تعمل بمطابقة التوقيعات دون إرسال التنبيهات إلى مركز عمليات أمنية مركزي، ولا تُوفّر أي استجابة آلية منسّقة أو رؤية موحّدة لحالة الأمن عبر الأجهزة."),
      spacer(60),
      labelParagraph("ثانياً — سجلات النظام المبعثرة", "تُخزَّن محلياً على كل جهاز دون جمع مركزي أو تحليل آلي، ويتطلب فحصها تدخلاً يدوياً من الفريق التقني في حال الاشتباه بحادثة، مما يجعل اكتشاف الانتشار الأفقي للتهديدات شبه مستحيل."),
      spacer(60),
      labelParagraph("ثالثاً — إجراءات يدوية للاستجابة", "تعتمد على تحقق الفريق التقني يدوياً بعد تلقي بلاغات المستخدمين أو ملاحظة سلوك غير طبيعي، مما يُطيل فترة الاختراق غير المكتشفة ويُضخّم حجم الضرر في كل حادثة أمنية."),
      spacer(100),

      // 2.3.2
      subHeading("2.3.2  تدفق العمل في الوضع القائم:"),
      spacer(40),

      arParagraph([
        { text: "يمكن توصيف تدفق العمل في الوضع القائم وفق المسار الآتي: يُصيب التهديد نقطة نهاية ويبدأ في الانتشار ← يلاحظ المستخدم سلوكاً غير طبيعي ويبلّغ الفريق التقني ← يتحقق الفريق يدوياً من الجهاز ويفحص السجلات المحلية ← يُعزَل الجهاز يدوياً إن ثبت الإصابة ← تبدأ عملية إعادة التهيئة. ويكشف هذا المسار عن ثغرة زمنية ضخمة بين وقوع الاختراق واكتشافه، تُقدَّر بأكثر من مئتي يوم في المتوسط في الأنظمة التقليدية [12]، وهو رقم يعكس حجم المخاطرة التي تواجهها المؤسسات التي لا تعتمد منظومات " },
        { text: "EDR", en: true },
        { text: " متكاملة." },
      ], { after: 160 }),

      // 2.3.3
      subHeading("2.3.3  القيود والمشكلات الرئيسة في الوضع القائم:"),
      spacer(40),

      arParagraph("تتجلى إشكاليات الوضع القائم في ستة محاور جوهرية تُشكّل جميعها الأساس الذي قامت عليه متطلبات المنظومة المُطوَّرة:", { after: 80 }),

      arBullet([
        { text: "العمى التشغيلي: ", bold: true },
        { text: "تعمل كل نقطة نهاية كجزيرة معزولة، ولا تتوفر رؤية مركزية لحالة الأجهزة، مما يستحيل معه رصد الانتشار الأفقي للتهديدات عبر شبكة المؤسسة." },
      ]),
      arBullet([
        { text: "التأخر الكبير في الاكتشاف: ", bold: true },
        { text: "يعتمد الاكتشاف على المستخدم النهائي؛ ومتوسط الفترة بين وقوع الاختراق واكتشافه يتجاوز مئتي يوم في الأنظمة التقليدية [12]، وهو ما يمنح المهاجم وقتاً كافياً لتحقيق التمركز وسرقة البيانات." },
      ]),
      arBullet([
        { text: "العجز أمام التهديدات المتقدمة: ", bold: true },
        { text: "لا تكتشف أدوات مكافحة الفيروسات هجمات " },
        { text: "(Living-off-the-Land)", en: true },
        { text: " التي تستغل أدوات شرعية كـ" },
        { text: "(PowerShell)", en: true },
        { text: " و" },
        { text: "(WMI)", en: true },
        { text: " دون إنشاء ملفات خبيثة قابلة للاكتشاف." },
      ]),
      arBullet([
        { text: "غياب سياق الحادث: ", bold: true },
        { text: "عند وقوع حادث، يضطر الفريق إلى إعادة بناء سلسلة الأحداث يدوياً من سجلات متبعثرة على أجهزة متعددة، مما يُطيل وقت التحليل ويُضعف دقته." },
      ]),
      arBullet([
        { text: "استجابة يدوية بطيئة: ", bold: true },
        { text: "تعتمد الاستجابة على تدخّل الإنسان في كل خطوة، مما يُطيل متوسط وقت الاحتواء " },
        { text: "(MTTC)", en: true },
        { text: " ويزيد حجم الأضرار في كل حادثة." },
      ]),
      arBullet([
        { text: "انعدام مسار التدقيق: ", bold: true },
        { text: "لا تُوثَّق إجراءات الاستجابة توثيقاً منهجياً مما يُضعف قدرة الامتثال للمعايير التنظيمية ويُعيق التعلم المؤسسي من الحوادث السابقة." },
      ]),
      spacer(160),

      // ══════════════════════════════════════════════════════════════════════
      // 2.4 دراسة الجدوى
      // ══════════════════════════════════════════════════════════════════════
      sectionHeading("2.4  دراسة الجدوى - Feasibility Study:"),
      spacer(60),

      arParagraph([
        { text: "تُقيّم دراسة الجدوى مدى قابلية المشروع للتنفيذ من الأبعاد التقنية والاقتصادية والتشغيلية، في إطار مشروع تخرج جامعي بكالوريوس يستهدف بناء منظومة " },
        { text: "EDR", en: true },
        { text: " مفتوحة المصدر. وقد استندت هذه الدراسة إلى نتائج مرحلة التقييم الموثّقة في الفصل الأول، وإلى التجربة العملية المتراكمة خلال مراحل التصميم والتطوير." },
      ], { after: 140 }),

      // 2.4.1
      subHeading("2.4.1  الجدوى التقنية - Technical Feasibility:"),
      spacer(40),

      arParagraph([
        { text: "اعتُمدت لغة " },
        { text: "(Go)", en: true },
        { text: " في تطوير جميع مكوّنات الخادم — البرنامج الطرفي ومدير الاتصال ومحرك الكشف ومحرك الاستجابة — لمزاياها الجوهرية في دعم التزامن العالي الأداء عبر نموذج " },
        { text: "Goroutines/Channels", en: true },
        { text: "، وتوليد ملفات تنفيذية منفردة تُيسّر النشر. أما لوحة التحكم فقد بُنيت بـ" },
        { text: "(TypeScript)", en: true },
        { text: " مع إطار " },
        { text: "(React 19)", en: true },
        { text: " وأداة البناء " },
        { text: "(Vite)", en: true },
        { text: "، مما يُوفّر واجهةً تفاعلية عالية الأداء." },
      ], { after: 100 }),

      arParagraph([
        { text: "يعتمد المشروع على " },
        { text: "(gRPC/Protocol Buffers)", en: true },
        { text: " للاتصال بين البرنامج الطرفي ومدير الاتصال، وعلى " },
        { text: "(Apache Kafka 7.5.0)", en: true },
        { text: " بثلاثة مواضيع: " },
        { text: "(events-raw)", en: true },
        { text: " بستة أقسام لاستقبال الأحداث، و" },
        { text: "(alerts)", en: true },
        { text: " بثلاثة أقسام، و" },
        { text: "(events-dlq)", en: true },
        { text: " لتخزين الأحداث الفاشلة ثلاثين يوماً. ويُستخدم " },
        { text: "(PostgreSQL 16)", en: true },
        { text: " للتخزين الدائم مع دعم " },
        { text: "(JSONB)", en: true },
        { text: " وفهرسة " },
        { text: "(GIN)", en: true },
        { text: "، و" },
        { text: "(Redis 7)", en: true },
        { text: " بحد أقصى 256 ميغابايت لخوارزمية " },
        { text: "(LRU)", en: true },
        { text: " للتخزين المؤقت. وتدلّ هذه التقنيات المعتمدة — وكلها مفتوحة المصدر وموثّقة وناضجة — على جدوى تقنية عالية." },
      ], { after: 100 }),

      arParagraph([
        { text: "تعتمد المنصة اعتماداً كاملاً على " },
        { text: "(Docker Compose)", en: true },
        { text: " لتغليف ثماني خدمات ونشرها بأمر واحد، مع فحوصات صحة " },
        { text: "(healthcheck)", en: true },
        { text: " وتبعيات مرتّبة لكل خدمة. وكون جميع التقنيات المعتمدة ناضجة وموثّقة ومجتمعاتها نشطة يُقلّص مخاطر الاعتماد التقني إلى حدودها الدنيا." },
      ], { after: 140 }),

      // 2.4.2
      subHeading("2.4.2  الجدوى الاقتصادية - Economic Feasibility:"),
      spacer(40),

      arParagraph("تتوزع تكاليف المشروع على المحاور الآتية، ويوضّح الجدول (2-2) التقدير التفصيلي لكل بند:", { after: 100 }),

      new Paragraph({
        bidi: true, alignment: AlignmentType.CENTER,
        spacing: { before: 80, after: 60 },
        children: [new TextRun({ text: "جدول (2-2): تقدير تكاليف المشروع", font: "Traditional Arabic", size: 24, bold: true, rtl: true, color: "1F3864" })],
      }),
      new Table({
        width: { size: 9026, type: WidthType.DXA },
        columnWidths: [3009, 3009, 3008],
        rows: [
          new TableRow({ children: [
            hCell("بند التكلفة", 3009),
            hCell("التفاصيل", 3009),
            hCell("التقدير", 3008),
          ]}),
          ...([
            ["تكاليف التطوير", "الجهد البشري لفريق التطوير (6 أعضاء)", "مُغطَّى بالجهد الطلابي"],
            ["البنية التحتية", "AWS EC2 t3.medium أو خادم جامعي محلي", "0 – 50 دولاراً / شهر"],
            ["البرمجيات", "تقنيات مفتوحة المصدر بالكامل", "صفر دولار"],
            ["الاختبار", "أجهزة افتراضية على الأجهزة الجامعية", "صفر تكلفة إضافية"],
          ]).map((row, i) => new TableRow({ children: [
            dCell(row[0], 3009, { bg: i % 2 === 0 ? "FFFFFF" : altRowBg }),
            dCell(row[1], 3009, { bg: i % 2 === 0 ? "FFFFFF" : altRowBg }),
            dCell(row[2], 3008, { bg: i % 2 === 0 ? "FFFFFF" : altRowBg }),
          ]})),
        ],
      }),
      spacer(80),
      arParagraph([
        { text: "يُتوقع أن تُقلّص المنظومة متوسط وقت الاحتواء من أيام إلى دقائق، وتُحرّر الكوادر التقنية من مهام الاستجابة اليدوية الروتينية، وتُقدّم بديلاً اقتصادياً فعّالاً لحلول " },
        { text: "EDR", en: true },
        { text: " التجارية التي تتجاوز تكاليف تراخيصها عشرات الآلاف من الدولارات سنوياً. وبذلك تتجاوز العوائد المتوقعة التكاليف الفعلية بمراحل." },
      ], { before: 100, after: 140 }),

      // 2.4.3
      subHeading("2.4.3  الجدوى التشغيلية - Operational Feasibility:"),
      spacer(40),

      arParagraph([
        { text: "صُمِّمت واجهة لوحة التحكم " },
        { text: "(SOC Dashboard)", en: true },
        { text: " وفق مبدأ التمحور حول المستخدم، وتوفر: لوحةً رئيسية بمؤشرات أداء رئيسية " },
        { text: "(KPIs)", en: true },
        { text: "، وجداول تنبيهات قابلة للفلترة بمعيار " },
        { text: "(MITRE ATT&CK)", en: true },
        { text: "، وأزرار استجابة مباشرة. كما يدعم النظام نموذج التحكم في الوصول المبني على الأدوار " },
        { text: "(RBAC)", en: true },
        { text: "، ولا يستلزم الانتقال إليه تغيير سلوك المستخدم النهائي، إذ يعمل البرنامج الطرفي في الخلفية بصمت تام. كما أن بنية " },
        { text: "(REST API)", en: true },
        { text: " المفتوحة تُتيح التكامل مع أدوات " },
        { text: "(SIEM)", en: true },
        { text: " أو أنظمة " },
        { text: "(ITSM)", en: true },
        { text: " القائمة دون الحاجة إلى إعادة هيكلة البنية التشغيلية للمؤسسة." },
      ], { after: 140 }),

      // 2.4.4
      subHeading("2.4.4  خلاصة دراسة الجدوى:"),
      spacer(40),

      arParagraph([
        { text: "تكشف الدراسة الثلاثية الأبعاد أن المشروع مجدٍ تقنياً بفضل انسجام التقنيات المختارة مع طبيعة المتطلبات، ومجدٍ اقتصادياً إذ تتجاوز عوائده المتوقعة تكاليفه بمراحل، ومجدٍ تشغيلياً نظراً لتصميمه وفق مبادئ سهولة الاستخدام والتكامل مع البيئات القائمة. ويُشكّل هذا التقييم الإيجابي تأكيداً على أن المضيّ في تطوير المنظومة قرارٌ سليم يجمع بين الواقعية التنفيذية والقيمة العلمية والعملية المضافة." },
      ], { after: 160 }),

      // ══════════════════════════════════════════════════════════════════════
      // 2.5 إدارة المخاطر
      // ══════════════════════════════════════════════════════════════════════
      sectionHeading("2.5  إدارة المخاطر - Risk Management:"),
      spacer(60),

      arParagraph([
        { text: "إدارة المخاطر ركيزةٌ أساسية في دورة حياة أي مشروع برمجي، وتكتسب أهميةً مضاعفةً في مشاريع الأمن السيبراني كمنظومة " },
        { text: "EDR", en: true },
        { text: "; إذ إن الإخفاق في تحديد مخاطر المشروع والتخطيط للتعامل معها لا يُؤثّر على جودة المنتج البرمجي فحسب، بل قد يُعرّض الأجهزة التي تُراقبها المنظومة وبيانات المؤسسات للخطر. وقد اعتُمد في هذا المشروع نهجٌ منظّم لإدارة المخاطر يمتد عبر جميع مراحل التطوير من التصميم المعماري حتى ما بعد النشر." },
      ], { after: 140 }),

      // 2.5.1
      subHeading("2.5.1  أهداف إدارة المخاطر ونطاقها:"),
      spacer(40),

      arParagraph("تسعى إدارة المخاطر في هذا المشروع إلى تحقيق خمسة أهداف متكاملة:", { after: 80 }),

      arBullet([{ text: "الرصد المبكر: ", bold: true }, { text: "تحديد المخاطر المحتملة في أقرب مرحلة ممكنة من دورة حياة التطوير قبل أن تتحوّل إلى مشكلات مكلفة المعالجة." }]),
      arBullet([{ text: "الأولوية الصحيحة: ", bold: true }, { text: "ترتيب المخاطر وفق أثرها واحتماليتها لتوجيه جهود الفريق نحو المعالجات ذات القيمة الأعلى أولاً." }]),
      arBullet([{ text: "الاستمرارية الأمنية: ", bold: true }, { text: "ضمان أن إخفاق أي مكوّن منفرد لا يُفضي إلى فقدان كامل للحماية، بل تُواصل المنظومة عملها بقدر مُقلَّص." }]),
      arBullet([{ text: "الموثوقية الوظيفية: ", bold: true }, { text: "ضمان عدم تأثير العيوب الهندسية على الدقة الأمنية، لا سيما معدلات الإيجابيات الكاذبة التي تُضعف ثقة المحللين بالتنبيهات." }]),
      arBullet([{ text: "الامتثال والشفافية: ", bold: true }, { text: "توثيق القرارات المتعلقة بالمخاطر لدعم الحوكمة الأكاديمية والتشغيلية، وتمكين الفريق من التعلم من أي انحرافات." }]),

      spacer(100),
      arParagraph([
        { text: "يشمل نطاق إدارة المخاطر جميع مكوّنات المنظومة الخمسة الموضّحة في القسم " },
        { text: "(1.2)", en: true },
        { text: " من الفصل الأول — البرنامج الطرفي ومدير الاتصال ومحرك الكشف ومحرك الاستجابة ولوحة التحكم — فضلاً عن البنية التحتية المساندة " },
        { text: "(PostgreSQL وKafka وRedis)", en: true },
        { text: "، وعملية النشر والتشغيل، ودورة حياة قواعد الكشف." },
      ], { after: 140 }),

      // 2.5.2
      subHeading("2.5.2  منهجية إدارة المخاطر المعتمدة:"),
      spacer(40),

      arParagraph("اعتُمد إطارٌ هجين يجمع بين أربع مرجعيات متكاملة لضمان شمولية التحليل ومنهجيته:", { after: 80 }),

      labelParagraph("أولاً — ISO 31000:2018", [
        "إطار إدارة المخاطر الدولي الذي يُحدّد دورةً متكاملةً تتضمن: تحديد السياق، وتقييم المخاطر (التحديد والتحليل والتقييم)، وعلاج المخاطر، والمراقبة والمراجعة. واعتُمدت هذه الدورة هيكلاً ناظماً لهذا القسم بأكمله.",
      ].join("")),
      spacer(60),
      labelParagraph("ثانياً — STRIDE", [
        "نموذج نمذجة التهديدات الصادر عن ",
        "(Microsoft Research)",
        " يُحلّل تهديدات كل مكوّن عبر ستة أبعاد: انتحال الهوية، والتلاعب بالبيانات، والإنكار، والإفصاح غير المُصرَّح، وتعطيل الخدمة، ورفع الصلاحيات. طُبّق على البرنامج الطرفي وقناة الاتصال ومحرك الكشف.",
      ].join("")),
      spacer(60),
      labelParagraph("ثالثاً — DREAD", "آلية كمّية لتقييم درجة المخاطر الأمنية عبر خمسة أبعاد: حجم الضرر المحتمل، وسهولة إعادة الإنتاج، وسهولة الاستغلال، ونطاق المتأثرين، واحتمالية الاكتشاف. استُعين بها في تقييم المخاطر الأمنية تحديداً."),
      spacer(60),
      labelParagraph("رابعاً — مصفوفة الاحتمالية-الأثر (5×5)", "مصفوفة الحرارة ذات الخمس مستويات تُنتج خمسة مستويات خطورة مُرمَّزة لونياً: منخفض جداً، ومنخفض، ومتوسط، ومرتفع، وحرج. تُشكّل الأداة المرجعية لتصنيف كل مخاطرة محددة."),
      spacer(140),

      // 2.5.3
      subHeading("2.5.3  تحديد المخاطر وتقييمها:"),
      spacer(40),

      arParagraph("جرى تحديد مخاطر المشروع عبر جلسات عصف ذهني منظّمة للفريق، مُدعَّمةً بمراجعة الأدبيات التقنية والكود المصدري لكل مكوّن. ويُلخّص الجدول (2-3) أبرز هذه المخاطر مع تقييمها:", { after: 100 }),

      new Paragraph({
        bidi: true, alignment: AlignmentType.CENTER,
        spacing: { before: 80, after: 60 },
        children: [new TextRun({ text: "جدول (2-3): تقييم مخاطر مشروع منصة EDR", font: "Traditional Arabic", size: 24, bold: true, rtl: true, color: "1F3864" })],
      }),
      new Table({
        width: { size: 9026, type: WidthType.DXA },
        columnWidths: [380, 2600, 1000, 1200, 1200, 1246, 1400],
        rows: [
          new TableRow({ children: [
            hCell("#", 380, true),
            hCell("المخاطرة", 2600),
            hCell("الفئة", 1000),
            hCell("الاحتمالية", 1200),
            hCell("شدة الأثر", 1200),
            hCell("مستوى الخطر", 1246),
            hCell("الاستراتيجية", 1400),
          ]}),
          ...[
            ["1", "فشل جلسة ETW", "تقنية", "متوسطة", "مرتفع جداً", "مرتفع", "تخفيف"],
            ["2", "فشل اتصال gRPC / خطأ mTLS", "تقنية", "متوسطة", "مرتفع", "مرتفع", "تخفيف"],
            ["3", "إنهاء عملية البرنامج الطرفي", "أمنية", "مرتفعة", "مرتفع جداً", "حرج", "تخفيف"],
            ["4", "اعتراض قناة gRPC", "أمنية", "منخفضة", "مرتفع جداً", "مرتفع", "تخفيف"],
            ["5", "تسرّب مفاتيح التشفير", "أمنية", "منخفضة جداً", "حرج", "مرتفع", "تجنّب"],
            ["6", "ارتفاع الإيجابيات الكاذبة", "أداء", "مرتفعة", "متوسط", "مرتفع", "تخفيف"],
            ["7", "إسقاط أحداث Kafka", "أداء", "متوسطة", "مرتفع", "مرتفع", "تخفيف"],
            ["8", "تراجع أداء PostgreSQL", "توسّع", "متوسطة", "مرتفع", "مرتفع", "تخفيف"],
            ["9", "إيقاف خدمات Docker", "تشغيلية", "متوسطة", "مرتفع", "مرتفع", "تخفيف"],
            ["10", "تأخر الجدول الزمني", "إدارية", "مرتفعة", "مرتفع", "مرتفع", "تخفيف"],
          ].map((row, i) => {
            const levelColor = row[5] === "حرج" ? "CC0000" : row[5] === "مرتفع" ? "D46B08" : "1F7A1F";
            return new TableRow({ children: [
              dCell(row[0], 380, { bg: i % 2 === 0 ? "FFFFFF" : altRowBg, en: true }),
              dCell(row[1], 2600, { bg: i % 2 === 0 ? "FFFFFF" : altRowBg }),
              dCell(row[2], 1000, { bg: i % 2 === 0 ? "FFFFFF" : altRowBg }),
              dCell(row[3], 1200, { bg: i % 2 === 0 ? "FFFFFF" : altRowBg }),
              dCell(row[4], 1200, { bg: i % 2 === 0 ? "FFFFFF" : altRowBg }),
              dCell(row[5], 1246, { bg: i % 2 === 0 ? "FFFFFF" : altRowBg, color: levelColor, bold: true }),
              dCell(row[6], 1400, { bg: i % 2 === 0 ? "FFFFFF" : altRowBg }),
            ]});
          }),
        ],
      }),
      spacer(100),

      // 2.5.4
      subHeading("2.5.4  آليات التخفيف التقنية المُنفَّذة:"),
      spacer(40),

      arParagraph("يتناول هذا القسم الضوابط التقنية المدمجة في الكود المصدري للمنظومة بهدف تخفيف المخاطر التشغيلية والأمنية الرئيسة. وقد رُتّبت هذه الضوابط وفق أولوية المخاطر التي تعالجها:", { after: 80 }),

      labelParagraph("أولاً — الحماية الذاتية للبرنامج الطرفي", "تُطبَّق طبقتان للحماية الذاتية: ضبط قائمة DACL على عملية البرنامج الطرفي بحيث لا تملك سوى حساب SYSTEM صلاحية PROCESS_TERMINATE، وتصليب قائمة DACL على خدمة Windows لمنع تنفيذ أوامر الإيقاف والحذف من موجّه الأوامر المرتفع الصلاحيات، مما يُحصّن المنظومة ضد أي محاولة للتعطيل."),
      spacer(60),
      labelParagraph("ثانياً — تشفير المفاتيح بـ DPAPI", "يُخزَّن مفتاح التشفير (AES-256-GCM) في ملف محمي بـ(Windows Data Protection API — DPAPI)، بحيث لا يمكن فكّ تشفيره إلا من قِبَل حساب SYSTEM على نفس الجهاز، مع حماية مسار الملف بصلاحيات 0700 لمنع أي وصول غير مُصرَّح به."),
      spacer(60),
      labelParagraph("ثالثاً — تشفير قناة الاتصال بـ mTLS", "تعتمد قناة gRPC التشفيرَ المتبادل (Mutual TLS — mTLS)، إذ تُصدر خدمة (certificate_service.go) شهادةً x.509 فريدةً لكل جهاز طرفي يمرّ بعملية التسجيل، مما يمنع أي طرف غير مُصادَق من الاتصال بالخادم أو انتحال هوية جهاز مُسجَّل."),
      spacer(60),
      labelParagraph("رابعاً — نظام التخزين المحلي WAL", "يُشغّل البرنامج الطرفي قائمةً دائمةً على القرص (bbolt key-value store) لتخزين الأحداث غير المُرسَلة. عند استعادة الاتصال تُعاد قراءتها وإرسالها بترتيب FIFO مع آلية (Exponential Backoff) لتجنّب إغراق الخادم."),
      spacer(60),
      labelParagraph("خامساً — نظام إلغاء تكرار التنبيهات", "يُطبّق محرك الكشف ذاكرةً مؤقتةً بمفتاح (ruleID + agentID) وزمن انتهاء صلاحية 60 ثانيةً لكل إدخال، مما يضمن توليد تنبيهٍ واحد كحدٍّ أقصى في الدقيقة لكل زوج (قاعدة - جهاز)، للحدّ من ضجيج التنبيهات."),
      spacer(60),
      labelParagraph("سادساً — فلترة جودة قواعد Sigma", "يُطبّق محرك الكشف عند تحميل قواعد Sigma فلترَ جودة ذا خمس مراحل يشمل: التحقق من وجود قسم detection، واشتراط توفّر العنوان والوصف، واستبعاد القواعد التجريبية عند الاقتضاء، والتقيّد بالحالات المسموح بها، وضبط حد أدنى لمستوى الخطورة."),
      spacer(140),

      // 2.5.5
      subHeading("2.5.5  مراقبة المخاطر ومراجعتها:"),
      spacer(40),

      arParagraph("تنقسم آليات مراقبة المخاطر إلى مرحلتين رئيسيتين متكاملتين:", { after: 80 }),

      labelParagraph("أولاً — خلال مرحلة التطوير", "تتضمن: مراجعات الكود في كل طلب دمج مع التحقق من صحة معالجة الأخطاء وسلامة الخيوط وإغلاق الموارد؛ واختبارات الوحدة للمكوّنات الحساسة؛ وإعادة تقييم لائحة المخاطر في نهاية كل دورة Sprint؛ وبيئة تكامل مستمر تكشف الانحدارات مبكراً مع كل commit."),
      spacer(60),
      labelParagraph("ثانياً — بعد النشر في بيئة التشغيل", "تتضمن: فحوصات صحة Docker كل 30 ثانيةً مع إعادة تشغيل تلقائية عند الفشل؛ ومؤشر statsReporter يُولّد تقريراً دورياً بمعدل الأحداث ومتوسط زمن الكشف وعدد التنبيهات؛ ومراقبة مؤشرات الأداء الرئيسية (MTTD وMTTC ومعدل فقدان الأحداث وWAL)؛ ومراجعة شاملة لسجل المخاطر كل ثلاثة أشهر."),
      spacer(100),

      arParagraph([
        { text: "يتكامل هذا النهج في إدارة المخاطر مع منهجية التطوير الرشيقة " },
        { text: "(Agile)", en: true },
        { text: " الموصوفة في القسم " },
        { text: "(1.8)", en: true },
        { text: " من الفصل الأول، إذ يُعاد تقييم لائحة المخاطر في كل دورة Sprint وتُحدَّث الأولويات بناءً على ما اكتشفه الفريق. كما تُؤثّر نتائج التقييم مباشرةً في أولويات قائمة المتراكمات " },
        { text: "(Product Backlog)", en: true },
        { text: "، بحيث تُعطى بنود معالجة المخاطر الحرجة أولويةً تفوق بنود الميزات غير الجوهرية في دورات التطوير." },
      ], { after: 200 }),

      // ══════════════════════════════════════════════════════════════════════
      // REFERENCES
      // ══════════════════════════════════════════════════════════════════════
      sectionHeading("المراجع - References:"),
      spacer(60),

      ...[
        "[1] Ponemon Institute, \"2022 Cost of a Data Breach Report,\" IBM Security, 2022.",
        "[2] A. Chuvakin, \"Endpoint Threat Detection and Response,\" Gartner Blog, 2013.",
        "[3] Wazuh, Inc., \"Wazuh Documentation,\" [Online]. Available: https://documentation.wazuh.com. [Accessed: 2024].",
        "[4] Xcitium (Comodo), \"OpenEDR GitHub Repository,\" [Online]. Available: https://github.com/ComodoSecurity/openedr. [Accessed: 2024].",
        "[5] M. Cohen, \"Velociraptor — Digging Deeper,\" Rapid7, [Online]. Available: https://www.velocidex.com. [Accessed: 2024].",
        "[6] OSSEC Project, \"OSSEC HIDS Documentation,\" [Online]. Available: https://www.ossec.net. [Accessed: 2024].",
        "[7] E. Rescorla, \"The Transport Layer Security (TLS) Protocol Version 1.3,\" RFC 8446, Internet Engineering Task Force, Aug. 2018.",
        "[8] Apache Software Foundation, \"Apache Kafka Documentation,\" [Online]. Available: https://kafka.apache.org/documentation. [Accessed: 2024].",
        "[9] T. Graeber, \"Event Tracing for Windows (ETW),\" Microsoft Docs, [Online]. Available: https://docs.microsoft.com/en-us/windows/win32/etw. [Accessed: 2024].",
        "[10] Florian Roth et al., \"Sigma: Generic Signature Format for SIEM Systems,\" [Online]. Available: https://github.com/SigmaHQ/sigma. [Accessed: 2024].",
        "[11] MITRE Corporation, \"ATT&CK Framework,\" [Online]. Available: https://attack.mitre.org. [Accessed: 2024].",
        "[12] IBM Security, \"X-Force Threat Intelligence Index 2023,\" IBM Corp., 2023.",
      ].map(ref => new Paragraph({
        alignment: AlignmentType.LEFT,
        spacing: { before: 40, after: 60 },
        children: [new TextRun({ text: ref, font: "Times New Roman", size: 22, color: "333333" })],
      })),

      spacer(200),
    ],
  }],
});

Packer.toBuffer(doc).then(buf => {
  fs.writeFileSync("chapter2.docx", buf);
  console.log("Done!");
});
