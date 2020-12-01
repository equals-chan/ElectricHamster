package net.lingala.zip4j;

import java.io.*;
import java.util.Enumeration;
import java.util.Properties;
import org.fusesource.jansi.AnsiConsole;

public class properties {
    private String folderPath;
    private String excelPath;
    private String sqlPath;
    private String zippedFilesPath;
    private String excelOpNum;


    properties(String path) throws IOException {

        InputStream in = null;
        Properties p = new Properties();
        //AnsiConsole.systemInstall();
        try {
            in = new BufferedInputStream(new FileInputStream(path+"config.properties"));
            p.load(in);

            System.out.println("\u001b[1;42m"+"已找到到配置文件，准备压缩。"+"\u001b[0m");
            Enumeration<Object> keys = p.keys();
            this.folderPath = (String) p.get("folderPath");
            this.excelPath = (String) p.get("excelPath");
            this.sqlPath = (String) p.get("sqlPath");
            this.zippedFilesPath = (String) p.get("zippedFilesPath");
            this.excelOpNum = (String) p.get("excelOpNum");
            while (keys.hasMoreElements()) {
                String key = (String) keys.nextElement();
                System.out.println(key + "：" + p.getProperty(key));
            }

        } catch (FileNotFoundException e) {
            // TODO Auto-generated catch block
            System.out.println("\u001b[1;41m 未检测到配置文件，请在新生成的配置文件中填写路径！！\u001b[0m");
            System.out.println("\u001b[1;41m 未检测到配置文件，请在新生成的配置文件中填写路径！！\u001b[0m");
            System.out.println("\u001b[1;41m 未检测到配置文件，请在新生成的配置文件中填写路径！！\u001b[0m");

            propertieCreater("config.properties");
            System.out.println("\u001b[1;45m 程序结束 \u001b[0m");
            System.exit(0);

        } catch (IOException e) {
            // TODO Auto-generated catch block
            e.printStackTrace();
        }

        //AnsiConsole.systemUninstall();


    }

    private void propertieCreater(String path) throws IOException {
        File file = new File(new File("./"),path);
        file.createNewFile();
        try (OutputStream out = new FileOutputStream(file);
             BufferedWriter bw = new BufferedWriter(new OutputStreamWriter(out, "utf-8"))
        ) {
            bw.write("#这是一个ElectricHamster的配置文件，请在这里填写本程序的所需配置\n");
            bw.write("#请保证路径文件夹已经存在，程序不会自动生成文件夹！\n\n");
            bw.write("#请注意！！路径要么写成D:\\\\a\\\\b\\\\c\\\\d\n");
            bw.write("#               要么写成D:/a/b/c/d\n");
            bw.write("#千万不要写成D:\\a\\b\\c\\d\n");
            bw.write("#不然神仙也就不了你！\n\n");
            bw.write("#下行为要压缩的文件路径\n");
            bw.write("folderPath=\n");
            bw.write("#下行为压缩文件的放置路径\n");
            bw.write("zippedFilesPath=\n");
            bw.write("\n#以下可不修改\n\n");
            bw.write("#下行为excel的放置路径,如无必要可不修改\n");
            bw.write("excelPath=./\n");
            bw.write("#下行为excel的生成类型，0为生成全部历史记录，1只生成本次压缩产生的条目！\n");
            bw.write("excelOpNum=0\n");
            bw.write("#下行为数据库文件的放置路径\n");
            bw.write("sqlPath=./\n");
            bw.flush();

        } catch (FileNotFoundException e) {
            e.printStackTrace();
        } catch (UnsupportedEncodingException e) {
            e.printStackTrace();
        } catch (IOException e) {
            e.printStackTrace();
        }

    }


    public static void main(String[] args) throws IOException {
        properties properties = new properties("config.properties");

    }


    public String getExcelPath() {
        return excelPath;
    }

    public String getFolderPath() {
        return folderPath;
    }

    public String getSqlPath() {
        return sqlPath;
    }

    public String getZippedFilesPath() {
        return zippedFilesPath;
    }

    public String getExcelOpNum() {
        return excelOpNum;
    }
}
