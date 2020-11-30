package net.lingala.zip4j;

import java.text.SimpleDateFormat;
import java.util.Date;

public class times {
    public static String getNowTimes() {
        Date date = new Date();
        SimpleDateFormat formatter = new SimpleDateFormat("yy-MM-dd HH:mm:ss");
        return formatter.format(date);
    }
}
