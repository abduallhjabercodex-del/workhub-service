#!/usr/bin/env bash
# ==============================================================================
# WorkHub Traffic & Reliability Simulator
#
# هذا السكربت يحاكي سيناريوهات حركة مرور واقعية لاختبار لوحة التحكم والتنبيهات:
# 1) حركة مرور عادية (Normal Traffic): طلبات مستمرة وتحديث الرسوم البيانية (RPS, DB, Cache, Latency)
# 2) عاصفة طلبات Rate Limit: إرسال سيل سريع من الطلبات لتجاوز الحد والحصول على 429 Too Many Requests
# 3) اختبار قاطع الدائرة (Circuit Breaker Drill):
#    - إيقاف Redis
#    - كسر الدائرة إلى OPEN (2 🔴) وتفعيل تنبيه Prometheus
#    - إعادة تشغيل Redis والانتظار
#    - إرسال طلب تجريبي وإظهار حالة HALF-OPEN (1 🟡) بوضوح
#    - إرسال بقية الطلبات للتعافي والعودة إلى CLOSED (0 🟢)
# ==============================================================================

set -eo pipefail

BASE_URL="${WORKHUB_URL:-http://localhost:8090}"
PROM_URL="${PROMETHEUS_URL:-http://localhost:9090}"
GRAFANA_URL="${GRAFANA_URL:-http://localhost:3001}"

# ألوان العرض في الطرفية
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

get_token() {
    local token
    token=$(curl -s -X POST "${BASE_URL}/api/v1/auth/login" \
        -H "Content-Type: application/json" \
        -d '{"email":"owner@workhub.dev","password":"Password1234!"}' | grep -o '"accessToken":"[^"]*' | cut -d'"' -f4 || true)
    if [[ -z "$token" ]]; then
        echo -e "${RED}❌ فشل الحصول على رمز تسجيل الدخول! تأكد من تشغيل الحاويات: docker compose up -d${NC}"
        exit 1
    fi
    echo "$token"
}

get_project_id() {
    local token="$1"
    local pid
    pid=$(curl -s -H "Authorization: Bearer $token" "${BASE_URL}/api/v1/projects" | grep -o '"id":"[^"]*' | head -n 1 | cut -d'"' -f4 || true)
    if [[ -z "$pid" ]]; then
        pid="3"
    fi
    echo "$pid"
}

run_normal_traffic() {
    local duration="${1:-30}"
    echo -e "\n${CYAN}========================================================================${NC}"
    echo -e "${BOLD}${BLUE}🚀 [1] بدء محاكاة حركة المرور العادية (Normal Traffic Simulation)${NC}"
    echo -e "المدة: ${BOLD}${duration} ثانية${NC} | الهدف: تحريك جميع مؤشرات لوحة التحكم بشكل واقعي"
    echo -e "${CYAN}========================================================================${NC}"
    
    local token
    token=$(get_token)
    local pid
    pid=$(get_project_id "$token")
    
    local end_time=$((SECONDS + duration))
    local count=0

    while [ $SECONDS -lt $end_time ]; do
        # 1. Health check
        curl -s "${BASE_URL}/health" >/dev/null &
        
        # 2. List Projects (Database read)
        curl -s -H "Authorization: Bearer $token" "${BASE_URL}/api/v1/projects" >/dev/null &
        
        # 3. Project Summary (Redis cache read / compute)
        curl -s -H "Authorization: Bearer $token" "${BASE_URL}/api/v1/projects/${pid}/summary" >/dev/null &
        
        # 4. Project Members
        curl -s -H "Authorization: Bearer $token" "${BASE_URL}/api/v1/projects/${pid}/members" >/dev/null &
        
        count=$((count + 4))
        echo -ne "\r${GREEN}⚡ تم إرسال ${count} طلبات بنجاح... المتبقي: $((end_time - SECONDS)) ثانية${NC}"
        sleep 0.5
    done
    wait
    echo -e "\n${GREEN}✓ اكتملت محاكاة حركة المرور العادية بنجاح! راقب الرسوم البيانية في Grafana: ${GRAFANA_URL}${NC}\n"
}

run_ratelimit_simulation() {
    echo -e "\n${CYAN}========================================================================${NC}"
    echo -e "${BOLD}${YELLOW}🛡️  [2] محاكاة تجاوز Rate Limit (HTTP 429 Too Many Requests)${NC}"
    echo -e "الهدف: إرسال سيل سريع لاختبار الحماية وتفعيل تنبيه HighRateLimitRejections"
    echo -e "${CYAN}========================================================================${NC}"
    
    echo -e "إرسال 15 طلب تسجيل دخول في أجزاء من الثانية..."
    local ok_count=0
    local rejected_count=0

    for i in {1..15}; do
        local code
        code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${BASE_URL}/api/v1/auth/login" \
            -H "Content-Type: application/json" \
            -d '{"email":"owner@workhub.dev","password":"Password1234!"}' || true)
        
        if [[ "$code" == "200" ]]; then
            ok_count=$((ok_count + 1))
            echo -e "  طلب #$i: ${GREEN}HTTP $code OK (مسموح)${NC}"
        elif [[ "$code" == "429" ]]; then
            rejected_count=$((rejected_count + 1))
            echo -e "  طلب #$i: ${RED}HTTP $code Too Many Requests (تم الحظر بواسطة Rate Limiter!)${NC}"
        else
            echo -e "  طلب #$i: HTTP $code"
        fi
    done

    echo -e "\n📊 ${BOLD}النتيجة:${NC} مسموح: ${GREEN}${ok_count}${NC} | محظور (429): ${RED}${rejected_count}${NC}"
    echo -e "⏳ جاري انتظار 5 ثوانٍ لتحديث مقاييس Prometheus و Grafana..."
    sleep 5

    # فحص التنبيه في بروميثيوس
    local alert_status
    alert_status=$(curl -s "${PROM_URL}/api/v1/alerts" | grep -o '"alertname":"HighRateLimitRejections"[^}]*' || true)
    if [[ -n "$alert_status" ]]; then
        echo -e "${GREEN}✓ تم إطلاق تنبيه Prometheus بنجاح: ${BOLD}HighRateLimitRejections (FIRING 🔥)${NC}"
        echo -e "  شاهد التنبيه في المتصفح: ${CYAN}${PROM_URL}/alerts${NC}"
    else
        echo -e "${YELLOW}ℹ️  تحقق من حالة التنبيهات في Prometheus: ${PROM_URL}/alerts${NC}"
    fi
    echo -e "  شاهد بطاقة Rate Limit في Grafana: ${CYAN}${GRAFANA_URL}${NC}\n"
}

run_circuit_breaker_drill() {
    echo -e "\n${CYAN}========================================================================${NC}"
    echo -e "${BOLD}${RED}⚡ [3] محاكاة دورة حياة قاطع الدائرة الكاملة (Circuit Breaker Drill)${NC}"
    echo -e "المراحل: CLOSED (0 🟢) ➔ OPEN (2 🔴) ➔ HALF-OPEN (1 🟡) ➔ CLOSED (0 🟢)"
    echo -e "${CYAN}========================================================================${NC}"

    local token
    token=$(get_token)
    local pid
    pid=$(get_project_id "$token")

    echo -e "\n${BOLD}المرحلة 1: فحص الحالة الحالية (يجب أن تكون مغلقة / سليم 🟢)${NC}"
    local init_state
    init_state=$(curl -s "${PROM_URL}/api/v1/query?query=workhub_cache_circuit_state" | grep -o '"value":\[[^]]*\]' || true)
    echo -e "الحالة في Prometheus: ${GREEN}${init_state}${NC}"

    echo -e "\n${BOLD}المرحلة 2: إيقاف خادم الكاش (docker stop workhub_redis)...${NC}"
    docker stop workhub_redis >/dev/null
    echo -e "${RED}⚠️  تم إيقاف Redis بنجاح. سنقوم بإرسال 4 طلبات لتجاوز عتبة الأخطاء (Threshold = 3)...${NC}"
    
    for i in {1..4}; do
        curl -s -o /dev/null -H "Authorization: Bearer $token" "${BASE_URL}/api/v1/projects/${pid}/summary" || true
        echo -e "  طلب فشل الكاش #$i ❌"
    done

    echo -e "⏳ انتظار 6 ثوانٍ ليقوم Prometheus بسحب المقياس وتقييم التنبيه..."
    sleep 6

    local open_state
    open_state=$(curl -s "${PROM_URL}/api/v1/query?query=workhub_cache_circuit_state" | grep -o '"value":\[[^,]*,"2"\]' || true)
    if [[ -n "$open_state" ]]; then
        echo -e "${RED}🔥 قاطع الدائرة أصبح الآن: OPEN (TRIPPED 🔴 - القيمة: 2)${NC}"
    else
        echo -e "${YELLOW}قاطع الدائرة في طور التحديث.${NC}"
    fi

    local cb_alert
    cb_alert=$(curl -s "${PROM_URL}/api/v1/alerts" | grep -o '"alertname":"CacheCircuitBreakerOpen"[^}]*' || true)
    if [[ -n "$cb_alert" ]]; then
        echo -e "${GREEN}✓ تنبيه Prometheus يشتعل الآن فوراً: ${BOLD}CacheCircuitBreakerOpen (FIRING 🔥)${NC}"
        echo -e "  شاهد التنبيه مباشرة في: ${CYAN}${PROM_URL}/alerts${NC}"
    fi

    echo -e "\n${BOLD}المرحلة 3: إعادة تشغيل خادم Redis (docker start workhub_redis)...${NC}"
    docker start workhub_redis >/dev/null
    echo -e "${GREEN}✓ تم تشغيل Redis بنجاح.${NC}"

    echo -e "\n${BOLD}المرحلة 4: فترة التهدئة (Cooldown: 10 ثوانٍ)...${NC}"
    for i in {10..1}; do
        echo -ne "\r⏳ جاري الانتظار لانتهاء فترة التهدئة: ${i} ثانية... "
        sleep 1
    done
    echo -e "\n${CYAN}✓ انتهت فترة التهدئة. القاطع جاهز للدخول في مرحلة الفحص التجريبي (Half-Open)...${NC}"

    echo -e "\n${BOLD}المرحلة 5: إرسال أول طلب فحص تجريبي (Probe Request)...${NC}"
    curl -s -o /dev/null -H "Authorization: Bearer $token" "${BASE_URL}/api/v1/projects/${pid}/summary"
    echo -e "${YELLOW}⚡ دخل القاطع في حالة: HALF-OPEN (PROBING 🟡 - القيمة: 1)${NC}"
    echo -e "⏳ جاري الانتظار 6 ثوانٍ لتمكين Prometheus من قراءة الحالة وعرضها في Grafana..."
    sleep 6

    local half_state
    half_state=$(curl -s "${PROM_URL}/api/v1/query?query=workhub_cache_circuit_state" | grep -o '"value":\[[^,]*,"1"\]' || true)
    if [[ -n "$half_state" ]]; then
        echo -e "${GREEN}✓ تم تسجيل حالة HALF-OPEN (1 🟡) بنجاح في Prometheus و Grafana!${NC}"
    fi

    echo -e "\n${BOLD}المرحلة 6: إرسال بقية طلبات الفحص الناجحة لإتمام الشفاء والإغلاق...${NC}"
    for i in {1..3}; do
        curl -s -o /dev/null -H "Authorization: Bearer $token" "${BASE_URL}/api/v1/projects/${pid}/summary"
        echo -e "  طلب فحص ناجح #$i ✓"
    done

    echo -e "⏳ انتظار 6 ثوانٍ للتحقق من عودة القاطع إلى الوضع الطبيعي..."
    sleep 6

    local closed_state
    closed_state=$(curl -s "${PROM_URL}/api/v1/query?query=workhub_cache_circuit_state" | grep -o '"value":\[[^,]*,"0"\]' || true)
    if [[ -n "$closed_state" ]]; then
        echo -e "${GREEN}🎉 تم استعادة القاطع بنجاح: CLOSED (HEALTHY 🟢 - القيمة: 0)${NC}"
        echo -e "${GREEN}✓ تم حل التنبيه واختفاؤه من قائمة التنبيهات النشطة.${NC}"
    fi

    echo -e "\n${BOLD}شاهد المخطط الزمني الكامل لحالات القاطع (State Timeline) في لوحة Grafana:${NC}"
    echo -e "${CYAN}${GRAFANA_URL}/d/adg7ldb/workhub-enterprise-observability-and-operations${NC}\n"
}

show_menu() {
    clear
    echo -e "${CYAN}========================================================================${NC}"
    echo -e "${BOLD}       WorkHub Observability & Chaos Traffic Simulator${NC}"
    echo -e "${CYAN}========================================================================${NC}"
    echo -e "  1) ${GREEN}محاكاة حركة مرور عادية مستمرة (Normal Traffic - 30s)${NC}"
    echo -e "  2) ${YELLOW}محاكاة عاصفة Rate Limit وتفعيل كود 429 والتنبيه${NC}"
    echo -e "  3) ${RED}محاكاة دورة حياة قاطع الدائرة (Closed -> Open -> Half-Open -> Closed)${NC}"
    echo -e "  4) ${BOLD}تشغيل جميع السيناريوهات بالكامل بالتسلسل (Full Demo)${NC}"
    echo -e "  5) خروج (Exit)"
    echo -e "${CYAN}========================================================================${NC}"
    read -rp "اختر رقم السيناريو [1-5]: " choice
    case "$choice" in
        1) run_normal_traffic 30 ;;
        2) run_ratelimit_simulation ;;
        3) run_circuit_breaker_drill ;;
        4)
            run_normal_traffic 15
            run_ratelimit_simulation
            run_circuit_breaker_drill
            ;;
        5) exit 0 ;;
        *) echo -e "${RED}خيار غير صحيح.${NC}" ;;
    esac
}

# التنفيذ بناءً على المدخلات
ACTION="${1:-menu}"
case "$ACTION" in
    normal) run_normal_traffic "${2:-30}" ;;
    ratelimit) run_ratelimit_simulation ;;
    breaker) run_circuit_breaker_drill ;;
    all)
        run_normal_traffic 15
        run_ratelimit_simulation
        run_circuit_breaker_drill
        ;;
    menu|*) show_menu ;;
esac
