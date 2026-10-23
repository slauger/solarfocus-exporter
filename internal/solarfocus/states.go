package solarfocus

import "strconv"

// Text of the enumerations from "Regelung ecomanager-touch: Modbus TCP - Registerdaten"
// (DR-0180-DE v1-201907). Only the tables for pellet/standard devices are included;
// therminator codes (offset 200) are reported as "code <n>".

// boilerStates is the status line of the boiler (Kessel), input register 2401.
var boilerStates = map[int]string{
	0:  "Bereitschaft",
	1:  "Zündphase",
	2:  "Pelletsbetrieb",
	3:  "Kesselsolltemperatur erreicht, Nachlauf",
	4:  "Nachlauf",
	5:  "keine Anforderung, Nachlauf",
	6:  "Brenner ausgeschaltet",
	7:  "Pelletsvorratsbehälter ist leer, Nachlauf",
	8:  "Wärmetauscherreinigung ist aktiv",
	9:  "Wärmetauscherreinigung benötigt, Nachlauf",
	10: "Stromausfall, Nachlauf",
	11: "zweiter Zündversuch, Nachlauf",
	12: "Füllraumtemperatur überschritten, Nachlauf",
	13: "Einschub überlastet, Nachlauf",
	14: "Fremdkessel aktiv, Nachlauf",
	15: "Fremdkessel aktiv, Bereitschaft",
	16: "Brenner ausgeschaltet, Nachlauf",
	17: "Sicherheitskette ist offen",
	18: "Sicherheitskette ist offen, Nachlauf",
	19: "Restsauerstoffgehalt zu hoch, Nachlauf",
	20: "Abgastemperatur zu gering, Nachlauf",
	21: "Abgastemperatur zu hoch, Bereitschaft",
	22: "Abgasfühler ist defekt, Bereitschaft",
	23: "Einschubtemperatur zu hoch, Bereitschaft",
	24: "Lambdasonde ist defekt, Nachlauf",
	25: "Kesseltemperatur ist ausreichend, Bereitschaft",
	26: "kein Stromfluss Einschubmotor, Nachlauf",
	27: "Kesselfühler ist defekt, Bereitschaft",
	28: "Kesselfühler ist defekt, Nachlauf",
	29: "Lambdasonde wird beheizt",
	30: "Wärmetauscherreinigung wartet auf eine Freigabe, Zeit",
	31: "Wärmetauscherreinigung wartet auf eine Freigabe, AGT",
	32: "Wärmetauscherreinigung wartet auf eine Freigabe, KT",
	33: "Saugaustragung wartet auf nächste Freigabezeit",
	34: "Pelletsvorratsbehälter wird befüllt",
	35: "Einschubfühlerkurzschluss, Bereitschaft",
	36: "Rückbrandschieber öffnet",
	37: "Kessel wird befüllt",
	38: "Lambdasonde wird kalibriert",
	39: "Kaminkehrer Messfreigabe",
	40: "Alarm aktiv!",
	41: "Kesseltür ist offen, Bereitschaft",
	42: "Tür war zu lange offen, Nachlauf",
	44: "keine Brennerfreigabe, Nachlauf",
	45: "Luftzahl Zündphase zu tief, Nachlauf",
	46: "Ascheaustragung aktiv",
	47: "Kesseltür/Aschebox ist offen, Bereitschaft",
	48: "Kesseltür/Aschebox wurde geöffnet, Nachlauf",
	49: "Kessel aktiv",
	50: "Saugaustragung wartet auf Umschalteinheit",
	51: "Brenner ausgeschaltet, Mindestkessellaufzeit aktiv",
	52: "Kaminkehrermessung beendet, Nachlauf",
	53: "warten bis die Raumluftklappe offen ist",
	54: "Stromausfall, Bereitschaft",
	55: "Stromausfall, Nachlauf",
	56: "WT-Spülung Brennwertmodul aktiv",
	57: "Differenzdruckschalter hat ausgelöst, Nachlauf",
	58: "warten auf Freigabe von Differenzdruckschalter",
	59: "WT-Spülung Brennwertmodul wartet auf eine Freigabe",
}

// heatingCircuitStates is the status of a heating circuit, input register 1107 (+50 per circuit).
var heatingCircuitStates = map[int]string{
	0:  "Heizkreis ist ausgeschaltet",
	1:  "Absenkbetrieb",
	2:  "Heizbetrieb",
	3:  "Ferienbetrieb",
	4:  "Estrichprogramm",
	5:  "Frostschutzbetrieb",
	6:  "Kaminkehrer",
	7:  "Heizkreis nicht freigeschaltet",
	8:  "Wärmeableitung",
	9:  "Außenabschalttemperatur Heizbetrieb erreicht",
	10: "Raumsolltemperatur Heizbetrieb erreicht",
	11: "Trinkwasserspeichervorrang ist aktiv",
	12: "Dauerheizbetrieb",
	13: "Dauerabsenkbetrieb",
	14: "Außenfühlerunterbrechung",
	15: "min. Energiequellentemperatur unterschritten",
	16: "Vorlauffühler defekt",
	17: "min. Energiequellentemperatur unterschritten, Frostschutzbetrieb",
	18: "Testlauf Pumpe ist aktiv",
	19: "Partybetrieb",
	20: "Begrenzungsthermostat ist offen",
	21: "Pumpen Nachlauf",
	22: "Defrost",
	23: "Kühlbetrieb",
	24: "Kühlen hat Vorrang",
	25: "Heizen hat Vorrang",
	26: "Pool hat Vorrang",
	27: "Außenabschalttemperatur Absenkbetrieb erreicht",
	28: "Raumsolltemperatur Absenkbetrieb erreicht",
}

// heatingCircuitModes is the operating mode of a heating circuit, holding register 32603.
var heatingCircuitModes = map[int]string{
	0: "Dauerbetrieb",
	1: "Absenkbetrieb",
	2: "Automatik",
	3: "Heizkreis ausgeschaltet",
}

// dhwStates is the status of a domestic hot water tank (Boiler), input register 501.
var dhwStates = map[int]string{
	0:  "Boilerstatus nicht vorhanden",
	1:  "Bereitschaft",
	2:  "Laden",
	3:  "Frostschutz",
	4:  "Rauchfangkehrermodus",
	5:  "Legionellenschutz",
	6:  "Anforderung",
	7:  "Energiequelle zu heiß",
	8:  "Blockadeschutz",
	9:  "einmalige Freigabe aktiv",
	10: "Fühler Kurzschluss",
	11: "Fühler Unterbrechung",
	12: "Ferienbetrieb",
	13: "Defrost",
}

// releaseModes is the release mode of a hot water tank (502 / 32002).
var releaseModes = map[int]string{
	0: "Immer Aus",
	1: "Immer Ein",
	2: "Montag - Sonntag",
	3: "Blockweise",
	4: "Tageweise",
}

// bufferStates is the status of a buffer tank, input register 1904 (+20 per buffer).
var bufferStates = map[int]string{
	0: "Status nicht vorhanden",
	1: "Bereitschaft",
	2: "Puffer wird beladen",
	3: "Frostschutzbetrieb",
	4: "Kaminkehrer",
	5: "Wärmeableitung",
	6: "Testlauf Pumpe ist aktiv",
	7: "Trinkwasserspeicher wird beladen",
}

func stateText(table map[int]string, code int) string {
	if s, ok := table[code]; ok {
		return s
	}
	return "code " + strconv.Itoa(code)
}
